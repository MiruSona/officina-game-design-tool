package conflict

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// BackendNLI 는 결과 줄의 backend 값이다. 판정 물음 종류(kind)로도 쓴다.
const BackendNLI = "nli"

// NLI 판정 서버 기다림 눈금 : health 1초 · 전체 min(5분, 쌍 × 1초 + 10초). 쌍당 약 0.2초(서버 README)에 여유를 둔 값이다.
const (
	HealthWait     = time.Second
	NLIWaitPerPair = time.Second
	NLIWaitBase    = 10 * time.Second
	nliLabelSet    = "nli4" // 응답 꼴을 손님 쪽에서 고정한다. 3칸 서버는 3칸으로 답한다
	nliSumSlack    = 0.02
	nliMaxBody     = 64 * 1024
)

// nliClient 는 프록시를 안 거치는 HTTP 손님이다. 주소는 설정에서 루프백만 받지만 환경 변수 프록시로 새는 길도 막는다.
// 리다이렉트도 안 따라간다 — 307/308 이면 같은 몸통(기획 글)을 루프백 밖 주소로 다시 보내게 된다.
var nliClient = &http.Client{
	Transport: &http.Transport{Proxy: nil},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// NLIWait 는 쌍 수에 맞춘 전체 기다림이다 : min(5분, 쌍 × 1초 + 10초).
func NLIWait(pairs int) time.Duration {
	d := time.Duration(pairs)*NLIWaitPerPair + NLIWaitBase
	if d > MaxWait {
		return MaxWait
	}
	return d
}

// healthReply 는 `GET /health` 답에서 읽는 칸이다.
type healthReply struct {
	OK    bool   `json:"ok"`
	Model string `json:"model"`
	Name  string `json:"name"`
}

// Health 는 판정 서버에 `/health` 를 1초 기다림으로 한 번 묻는다. 서버가 살아 있으면 모델 id 와 이름을 돌려준다.
func Health(baseURL string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), HealthWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(baseURL, "/health"), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := nliClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("health HTTP %d", resp.StatusCode)
	}
	var h healthReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, nliMaxBody)).Decode(&h); err != nil {
		return "", "", fmt.Errorf("health 답을 못 읽음 : %v", err)
	}
	if !h.OK {
		return "", "", errors.New("health 가 ok 가 아님")
	}
	return h.Model, h.Name, nil
}

// nliRequest 는 `POST /judge` 몸통이다.
type nliRequest struct {
	Evidence string `json:"evidence"`
	Claim    string `json:"claim"`
	LabelSet string `json:"label_set"`
}

// nliReply 는 `/judge` 답에서 읽는 칸이다. a·b·c 는 꼭 있어야 하고 d 는 4칸 서버일 때만 온다.
type nliReply struct {
	A     *float64 `json:"a"`
	B     *float64 `json:"b"`
	C     *float64 `json:"c"`
	D     *float64 `json:"d"`
	Model string   `json:"model"`
}

// nliLine 은 결과 jsonl 한 줄이다. localharness 결과 줄 꼴에 backend · model 을 더한 것이다.
type nliLine struct {
	ID      string             `json:"id"`
	Letter  string             `json:"letter,omitempty"`
	Prob    float64            `json:"prob,omitempty"`
	Probs   map[string]float64 `json:"probs,omitempty"`
	MS      int64              `json:"ms"`
	Problem string             `json:"problem,omitempty"`
	Backend string             `json:"backend"`
	Model   string             `json:"model"`
}

// RunNLI 는 줄마다 판정 서버에 묻는다. 줄 하나가 실패하면 그 줄의 Problem 에 남기고 다음 줄로 간다.
// 전체 기다림을 넘기면 남은 줄은 모두 Problem 이 되고 ExitErr 에 까닭이 찍힌다.
func RunNLI(baseURL string, rows []Row, wait time.Duration) Outcome {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	oc := Outcome{Verdicts: map[string]Verdict{}}
	for _, r := range rows {
		started := time.Now()
		v, model := askNLI(ctx, baseURL, r)
		v.ID = r.ID
		v.MS = time.Since(started).Milliseconds()
		if ctx.Err() != nil && v.Problem != "" {
			v.Problem = fmt.Sprintf("판정 서버가 %s 안에 안 끝났습니다", wait)
		}
		oc.Verdicts[r.ID] = v
		raw, _ := json.Marshal(nliLine{ID: v.ID, Letter: v.Letter, Prob: v.Prob, Probs: v.Probs, MS: v.MS,
			Problem: v.Problem, Backend: BackendNLI, Model: model})
		oc.Accepted = append(oc.Accepted, raw...)
		oc.Accepted = append(oc.Accepted, '\n')
	}
	if ctx.Err() != nil {
		oc.ExitErr = fmt.Sprintf("판정 서버가 %s 안에 안 끝났습니다", wait)
	}
	return oc
}

// askNLI 는 한 줄을 묻고 판정과 답한 모델 id 를 돌려준다. 실패는 Verdict.Problem 에 담는다.
func askNLI(ctx context.Context, baseURL string, r Row) (Verdict, string) {
	body, err := json.Marshal(nliRequest{Evidence: r.Evidence, Claim: r.Claim, LabelSet: nliLabelSet})
	if err != nil {
		return Verdict{Problem: "요청을 못 만듦 : " + err.Error()}, ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(baseURL, "/judge"), bytes.NewReader(body))
	if err != nil {
		return Verdict{Problem: "요청을 못 만듦 : " + err.Error()}, ""
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := nliClient.Do(req)
	if err != nil {
		return Verdict{Problem: "판정 서버 오류 : " + err.Error()}, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Verdict{Problem: fmt.Sprintf("판정 서버 HTTP %d", resp.StatusCode)}, ""
	}
	var rep nliReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, nliMaxBody)).Decode(&rep); err != nil {
		return Verdict{Problem: "판정 서버 답을 못 읽음 : " + err.Error()}, ""
	}
	probs, problem := nliProbs(rep)
	if problem != "" {
		return Verdict{Problem: problem}, rep.Model
	}
	letter, top := topLetter(probs)
	return Verdict{Letter: letter, Prob: top, Probs: probs}, rep.Model
}

// nliProbs 는 답의 칸을 판정 글자로 옮긴다 : a → A(지지) · b → B(반대) · c → C(무관) · d → R(같은 대상 · 적어 두기만).
// 칸이 빠졌거나 0~1 밖이거나 있는 칸의 합이 1±0.02 밖이면 두 번째 값에 까닭을 적는다.
func nliProbs(rep nliReply) (map[string]float64, string) {
	if rep.A == nil || rep.B == nil || rep.C == nil {
		return nil, "판정 서버 답에 a·b·c 칸이 빠짐"
	}
	probs := map[string]float64{"A": *rep.A, "B": *rep.B, "C": *rep.C}
	if rep.D != nil {
		probs["R"] = *rep.D
	}
	sum := 0.0
	for _, p := range probs {
		if math.IsNaN(p) || p < 0 || p > 1 {
			return nil, "판정 서버 답의 확률이 0~1 밖"
		}
		sum += p
	}
	if math.Abs(sum-1) > nliSumSlack {
		return nil, fmt.Sprintf("판정 서버 답의 확률 합이 %.3f", sum)
	}
	return probs, ""
}

// topLetter 는 가장 큰 칸의 글자와 확률이다. 같으면 A · B · C · R 차례로 앞선 것을 고른다.
func topLetter(probs map[string]float64) (string, float64) {
	best, top := "", -1.0
	for _, letter := range []string{"A", "B", "C", "R"} {
		p, ok := probs[letter]
		if ok && p > top {
			best, top = letter, p
		}
	}
	return best, top
}

// endpoint 는 서버 주소 뒤에 경로를 붙인다. 주소 끝의 `/` 는 하나로 접는다.
func endpoint(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + path
}

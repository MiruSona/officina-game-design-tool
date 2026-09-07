// Package templates 는 실행 파일 안에 넣는 HTML 틀을 담는다. 바깥 파일에 기대지 않는다.
package templates

import "embed"

// FS 는 틀 파일 묶음이다.
//
//go:embed dash.html
var FS embed.FS

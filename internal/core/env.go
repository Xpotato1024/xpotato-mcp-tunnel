package core

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This is a deliberately small literal data format, not a shell/dotenv runner.
// It never consults or changes process environment variables.
func ReadSetupEnv(file string) (Options, error) {
	var o Options
	if strings.ContainsAny(file, "\x00\r\n") {
		return o, fmt.Errorf(".env のパスが不正です")
	}
	f, e := os.Open(file)
	if e != nil {
		return o, fmt.Errorf(".env ファイルを開けません（内容は表示しません）")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return o, fmt.Errorf(".env は 64 KiB 以下の通常ファイルにしてください")
	}
	b, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil || len(b) > 65536 || !utf8.Valid(b) || bytes.ContainsRune(b, '\x00') {
		return o, fmt.Errorf(".env は NUL を含まない UTF-8、64 KiB 以下にしてください")
	}
	o, e = parseSetupEnv(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}))
	if e != nil {
		return Options{}, e
	}
	o.EnvFile = file
	if e = validateEnvRoots(o.Allow); e != nil {
		return Options{}, e
	}
	return o, nil
}

func parseSetupEnv(b []byte) (Options, error) {
	var o Options
	values := map[string]string{}
	dirs := map[int]string{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	line := 0
	for scanner.Scan() {
		line++
		s := strings.TrimSpace(scanner.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok {
			return o, fmt.Errorf(".env %d 行目: KEY=VALUE 形式が必要です（値は表示しません）", line)
		}
		index := 0
		if strings.HasPrefix(k, "ALLOW_DIR_") {
			n, err := strconv.Atoi(strings.TrimPrefix(k, "ALLOW_DIR_"))
			if err == nil && n >= 1 && n <= 100 && k == "ALLOW_DIR_"+strconv.Itoa(n) {
				index = n
			}
		}
		if k != "DEVICE" && k != "TUNNEL_ID" && index == 0 {
			return o, fmt.Errorf(".env %d 行目: DEVICE、TUNNEL_ID、ALLOW_DIR_1..100 のみ使用できます。API キーは保存・読み込みできません", line)
		}
		if _, duplicate := values[k]; duplicate {
			return o, fmt.Errorf(".env %d 行目: 同じ設定名を重複指定しています", line)
		}
		if strings.HasPrefix(v, "'") {
			if len(v) < 2 || !strings.HasSuffix(v, "'") {
				return o, fmt.Errorf(".env %d 行目: 単引用符を閉じてください", line)
			}
			v = v[1 : len(v)-1]
		} else if strings.ContainsAny(v, "'\"") {
			return o, fmt.Errorf(".env %d 行目: 値は単引用符で囲むか引用符なしで指定してください", line)
		}
		if v == "" || strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return o, fmt.Errorf(".env %d 行目: 空欄・制御文字は使用できません", line)
		}
		values[k] = v
		if index != 0 {
			dirs[index] = v
		}
	}
	if scanner.Err() != nil {
		return o, fmt.Errorf(".env の行が長すぎるか読み取れません")
	}
	o.Device, o.Tunnel = values["DEVICE"], values["TUNNEL_ID"]
	if !namePattern.MatchString(o.Device) || !tunnelPattern.MatchString(o.Tunnel) || len(dirs) == 0 {
		return o, fmt.Errorf(".env に正しい DEVICE、TUNNEL_ID、ALLOW_DIR_1 以降を指定してください（値は表示しません）")
	}
	for n := 1; n <= len(dirs); n++ {
		p, ok := dirs[n]
		if !ok {
			return o, fmt.Errorf(".env の ALLOW_DIR は 1 から欠番なく指定してください")
		}
		o.Allow = append(o.Allow, p)
	}
	return o, nil
}

func validateEnvRoots(roots []string) error {
	if len(roots) == 0 {
		return fmt.Errorf("ALLOW_DIR は一つ以上必要です")
	}
	seen := map[string]bool{}
	for n, p := range roots {
		if !validRoot(p) {
			return fmt.Errorf("ALLOW_DIR_%d: 存在するフォルダーの絶対パスが必要です（値は表示しません）", n+1)
		}
		p = filepath.Clean(p)
		if runtime.GOOS == "windows" {
			p = strings.ToLower(p)
		}
		if seen[p] {
			return fmt.Errorf("ALLOW_DIR_%d: 同じフォルダーが重複しています", n+1)
		}
		seen[p] = true
	}
	return nil
}

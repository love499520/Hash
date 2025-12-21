package source

import (
	"fmt"
	"strconv"
	"strings"
)

// getByPath：极简 JSONPath（dot + 数组下标）
// 支持：a.b.c  / a.b[0].c
func getByPath(root any, path string) (any, error) {
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}
	cur := root
	parts := strings.Split(path, ".")
	for _, p := range parts {
		if p == "" {
			continue
		}

		// 处理数组：x[0]
		name := p
		idx := -1
		if strings.Contains(p, "[") && strings.HasSuffix(p, "]") {
			l := strings.Index(p, "[")
			name = p[:l]
			n := p[l+1 : len(p)-1]
			i, err := strconv.Atoi(n)
			if err != nil {
				return nil, fmt.Errorf("bad index: %s", p)
			}
			idx = i
		}

		if name != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("not object at %s", name)
			}
			cur, ok = m[name]
			if !ok {
				return nil, fmt.Errorf("missing key %s", name)
			}
		}

		if idx >= 0 {
			arr, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("not array at %s", p)
			}
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("index out of range %s", p)
			}
			cur = arr[idx]
		}
	}
	return cur, nil
}

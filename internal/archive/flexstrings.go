package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// FlexStrings decodes a JSON field that the Internet Archive may return as
// null, a single string, or an array of strings. IA's advanced-search API
// collapses single-valued metadata fields to a bare string and keeps
// multi-valued ones as arrays, so any field that is conceptually a list
// (creator, subject, language, sometimes title/description/date) has to
// tolerate both shapes.
type FlexStrings []string

func (f *FlexStrings) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*f = nil
		return nil
	}
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexStrings{s}
		return nil
	case '[':
		var raw []interface{}
		if err := json.Unmarshal(b, &raw); err != nil {
			return err
		}
		out := make(FlexStrings, 0, len(raw))
		for _, v := range raw {
			if v == nil {
				continue
			}
			if s, ok := v.(string); ok {
				out = append(out, s)
				continue
			}
			out = append(out, fmt.Sprint(v))
		}
		*f = out
		return nil
	}
	return fmt.Errorf("FlexStrings: unexpected JSON token %q", string(b))
}

func (f FlexStrings) First() string {
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func (f FlexStrings) Join(sep string) string {
	return strings.Join(f, sep)
}

package zentao

import (
	"encoding/json"
	"regexp"
)

var (
	bugIDRe    = regexp.MustCompile(`\[#([0-9]+)::`)
	urlBugIDRe = regexp.MustCompile(`[?&]id=([0-9]+)`)
)

// ID 兼容禅道 webhook 中 objectID 为字符串或数字两种情况。
type ID string

func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*id = ID(s)
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*id = ID(n.String())
	return nil
}

// Webhook 禅道 webhook 请求体。
type Webhook struct {
	ObjectType string `json:"objectType"`
	ObjectID   ID     `json:"objectID"`
	Action     string `json:"action"`
	Actor      string `json:"actor"`
	Date       string `json:"date"`
	Comment    string `json:"comment"`
	Text       string `json:"text"`
}

// ParseWebhook 解析禅道 webhook 请求体。
func ParseWebhook(body []byte) (Webhook, error) {
	var hook Webhook
	if err := json.Unmarshal(body, &hook); err != nil {
		return Webhook{}, err
	}
	return hook, nil
}

// BugID 从 webhook 文本或 objectID 中提取 Bug ID。
func (w Webhook) BugID() string {
	if id := ExtractBugID(w.Text); id != "" {
		return id
	}
	return string(w.ObjectID)
}

// ExtractBugID 从 webhook 文本中提取 Bug ID。
func ExtractBugID(text string) string {
	if m := bugIDRe.FindStringSubmatch(text); len(m) == 2 {
		return m[1]
	}
	if m := urlBugIDRe.FindStringSubmatch(text); len(m) == 2 {
		return m[1]
	}
	return ""
}

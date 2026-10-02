package translate

import "strings"

// Many open models (DeepSeek, Qwen, GLM) put their reasoning in the content,
// between <think> and </think>. thinkSplitter sends that reasoning to one
// callback and the rest to another, and holds back a tag cut in two by a chunk
// boundary ("<thi" then "nk>") until the next chunk settles it.
type thinkSplitter struct {
	carry string
	in    bool
}

var thinkTags = []string{"<think>", "</think>", "<thinking>", "</thinking>"}

// push feeds one chunk of content.
func (s *thinkSplitter) push(raw string, onThink, onText func(string)) {
	if raw == "" {
		return
	}
	buf := s.carry + raw
	s.carry = ""
	if i := strings.LastIndexByte(buf, '<'); i >= 0 {
		tail := strings.ToLower(buf[i:])
		if !strings.Contains(tail, ">") && isThinkTagPrefix(tail) {
			s.carry, buf = buf[i:], buf[:i]
		}
	}
	for buf != "" {
		i := strings.IndexByte(buf, '<')
		if i < 0 {
			s.emit(buf, onThink, onText)
			return
		}
		s.emit(buf[:i], onThink, onText)
		buf = buf[i:]
		if tag, open := thinkTagAt(buf); tag > 0 {
			s.in = open
			buf = buf[tag:]
			continue
		}
		s.emit("<", onThink, onText)
		buf = buf[1:]
	}
}

// flush releases what was held back once the content has ended.
func (s *thinkSplitter) flush(onThink, onText func(string)) {
	c := s.carry
	s.carry = ""
	s.emit(c, onThink, onText)
}

func (s *thinkSplitter) emit(t string, onThink, onText func(string)) {
	switch {
	case t == "":
	case s.in:
		onThink(t)
	default:
		onText(t)
	}
}

func isThinkTagPrefix(tail string) bool {
	for _, t := range thinkTags {
		if strings.HasPrefix(t, tail) {
			return true
		}
	}
	return false
}

// thinkTagAt reports the length of a think tag at the start of s, and whether
// it opens the reasoning; 0 when s does not start with one.
func thinkTagAt(s string) (n int, open bool) {
	low := strings.ToLower(s)
	for _, t := range thinkTags {
		if strings.HasPrefix(low, t) {
			return len(t), !strings.HasPrefix(t, "</")
		}
	}
	return 0, false
}

// splitThinkTags splits a whole content into its reasoning and its text.
func splitThinkTags(content string) (think, text string) {
	var th, tx strings.Builder
	var s thinkSplitter
	onThink, onText := func(t string) { th.WriteString(t) }, func(t string) { tx.WriteString(t) }
	s.push(content, onThink, onText)
	s.flush(onThink, onText)
	return th.String(), tx.String()
}

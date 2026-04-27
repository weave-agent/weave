package skills

import (
	"strings"
)

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")

	return s
}

func formatSkillsPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")

	for _, s := range skills {
		b.WriteString("<skill>\n<name>")
		b.WriteString(escapeXML(s.Name))
		b.WriteString("</name>\n<description>")
		b.WriteString(escapeXML(s.Description))
		b.WriteString("</description>\n<location>")
		b.WriteString(escapeXML(s.FilePath))
		b.WriteString("</location>\n</skill>\n")
	}

	b.WriteString("</available_skills>")

	return b.String()
}

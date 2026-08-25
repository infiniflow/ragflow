package common

import "strings"

const wikiTopicPathSeparator = "/"

// GeneralWikiTopic is the fallback topic for pages that cannot be assigned to
// a topic emitted by the MAP stage.
const GeneralWikiTopic = "General"

// NormalizeWikiTopicPath canonicalizes a materialized Wiki topic path while
// preserving its human-readable segments. A one-segment topic is a valid root
// path; empty segments are discarded.
func NormalizeWikiTopicPath(topic string) string {
	parts := strings.Split(topic, wikiTopicPathSeparator)
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, wikiTopicPathSeparator)
}

// WikiTopicLeaf returns the last segment of a normalized Wiki topic path.
func WikiTopicLeaf(topic string) string {
	topic = NormalizeWikiTopicPath(topic)
	if topic == "" {
		return ""
	}
	if i := strings.LastIndex(topic, wikiTopicPathSeparator); i >= 0 {
		return topic[i+1:]
	}
	return topic
}

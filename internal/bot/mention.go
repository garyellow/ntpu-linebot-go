// Package bot provides the core bot logic and message processing.
package bot

import (
	"slices"
	"strings"

	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

// isBotMentioned checks if the bot is mentioned in a text message.
// It iterates through all mentionees and checks if any is a UserMentionee with IsSelf == true.
// Returns false if the message has no mentions or the bot is not mentioned.
func isBotMentioned(textMsg webhook.TextMessageContent) bool {
	if textMsg.Mention == nil || len(textMsg.Mention.Mentionees) == 0 {
		return false
	}

	for _, mentionee := range textMsg.Mention.Mentionees {
		if userMentionee, ok := mentionee.(webhook.UserMentionee); ok {
			if userMentionee.IsSelf {
				return true
			}
		}
	}

	return false
}

// mentionInfo holds the index and length of a mention to be removed.
type mentionInfo struct {
	index  int32
	length int32
}

// removeBotMentions removes all bot mentions from the text.
// It uses the Index and Length fields from UserMentionee where IsSelf == true.
// Mentions are removed from back to front to preserve index validity.
// The resulting text has extra whitespace normalized.
func removeBotMentions(text string, mention *webhook.Mention) string {
	if mention == nil || len(mention.Mentionees) == 0 {
		return text
	}

	// Collect all bot mentions (IsSelf == true)
	var botMentions []mentionInfo
	for _, mentionee := range mention.Mentionees {
		if userMentionee, ok := mentionee.(webhook.UserMentionee); ok {
			if userMentionee.IsSelf {
				botMentions = append(botMentions, mentionInfo{
					index:  userMentionee.Index,
					length: userMentionee.Length,
				})
			}
		}
	}

	if len(botMentions) == 0 {
		return text
	}

	// Sort by index descending to remove from back to front
	slices.SortFunc(botMentions, func(a, b mentionInfo) int {
		return int(b.index - a.index)
	})

	// Convert text to runes for proper UTF-8 handling
	// LINE SDK uses character index (runes), not byte index
	runes := []rune(text)

	for _, m := range botMentions {
		startIdx := int(m.index)
		endIdx := int(m.index + m.length)

		// Bounds check
		if startIdx < 0 {
			startIdx = 0
		}
		if endIdx > len(runes) {
			endIdx = len(runes)
		}
		if startIdx >= endIdx || startIdx >= len(runes) {
			continue
		}

		// Remove the mention by slicing
		runes = append(runes[:startIdx], runes[endIdx:]...)
	}

	// Convert back to string and normalize whitespace
	// strings.Fields splits on any whitespace and Join with single space
	return strings.Join(strings.Fields(string(runes)), " ")
}

// Package webhook provides LINE webhook handling and message dispatching.
// This file contains functions for detecting and processing @Bot mentions.
package webhook

import (
	"sort"
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
	sort.Slice(botMentions, func(i, j int) bool {
		return botMentions[i].index > botMentions[j].index
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
	result := string(runes)
	result = normalizeWhitespaceForMention(result)

	return result
}

// normalizeWhitespaceForMention collapses multiple whitespace characters into single spaces
// and trims leading/trailing whitespace. This is used after removing mentions.
func normalizeWhitespaceForMention(s string) string {
	// Replace multiple spaces with single space
	var builder strings.Builder
	builder.Grow(len(s))

	prevWasSpace := true // Start true to trim leading spaces
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevWasSpace {
				builder.WriteRune(' ')
				prevWasSpace = true
			}
		} else {
			builder.WriteRune(r)
			prevWasSpace = false
		}
	}

	result := builder.String()
	// Trim trailing space if present
	if len(result) > 0 && result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}

	return result
}

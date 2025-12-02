package webhook

import (
	"testing"

	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

func TestIsBotMentioned(t *testing.T) {
	tests := []struct {
		name     string
		textMsg  webhook.TextMessageContent
		expected bool
	}{
		{
			name: "bot is mentioned with IsSelf true",
			textMsg: webhook.TextMessageContent{
				Text: "@Bot hello",
				Mention: &webhook.Mention{
					Mentionees: []webhook.MentioneeInterface{
						webhook.UserMentionee{
							Index:  0,
							Length: 4,
							IsSelf: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "other user is mentioned, not bot",
			textMsg: webhook.TextMessageContent{
				Text: "@User hello",
				Mention: &webhook.Mention{
					Mentionees: []webhook.MentioneeInterface{
						webhook.UserMentionee{
							Index:  0,
							Length: 5,
							IsSelf: false,
							UserId: "U1234567890",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "multiple mentions, one is bot",
			textMsg: webhook.TextMessageContent{
				Text: "@User @Bot hello",
				Mention: &webhook.Mention{
					Mentionees: []webhook.MentioneeInterface{
						webhook.UserMentionee{
							Index:  0,
							Length: 5,
							IsSelf: false,
							UserId: "U1234567890",
						},
						webhook.UserMentionee{
							Index:  6,
							Length: 4,
							IsSelf: true,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "all mention type (not user)",
			textMsg: webhook.TextMessageContent{
				Text: "@All hello",
				Mention: &webhook.Mention{
					Mentionees: []webhook.MentioneeInterface{
						webhook.AllMentionee{
							Index:  0,
							Length: 4,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "no mention",
			textMsg: webhook.TextMessageContent{
				Text:    "hello world",
				Mention: nil,
			},
			expected: false,
		},
		{
			name: "empty mentionees slice",
			textMsg: webhook.TextMessageContent{
				Text: "hello",
				Mention: &webhook.Mention{
					Mentionees: []webhook.MentioneeInterface{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBotMentioned(tt.textMsg)
			if result != tt.expected {
				t.Errorf("isBotMentioned() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestRemoveBotMentions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		mention  *webhook.Mention
		expected string
	}{
		{
			name: "single bot mention at start",
			text: "@Bot hello world",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  0,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "hello world",
		},
		{
			name: "single bot mention in middle",
			text: "hello @Bot world",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  6,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "hello world",
		},
		{
			name: "single bot mention at end",
			text: "hello world @Bot",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  12,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "hello world",
		},
		{
			name: "multiple bot mentions",
			text: "@Bot hello @Bot world @Bot",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  0,
						Length: 4,
						IsSelf: true,
					},
					webhook.UserMentionee{
						Index:  11,
						Length: 4,
						IsSelf: true,
					},
					webhook.UserMentionee{
						Index:  22,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "hello world",
		},
		{
			name: "mixed mentions - only remove bot",
			text: "@User @Bot hello",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  0,
						Length: 5,
						IsSelf: false,
						UserId: "U1234567890",
					},
					webhook.UserMentionee{
						Index:  6,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "@User hello",
		},
		{
			name:     "no mention object",
			text:     "hello world",
			mention:  nil,
			expected: "hello world",
		},
		{
			name: "empty mentionees",
			text: "hello world",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{},
			},
			expected: "hello world",
		},
		{
			name: "only all mentionee (not bot)",
			text: "@All hello",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.AllMentionee{
						Index:  0,
						Length: 4,
					},
				},
			},
			expected: "@All hello",
		},
		{
			name: "CJK characters with bot mention",
			text: "@Bot 查詢課程",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  0,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "查詢課程",
		},
		{
			name: "CJK with bot mention in middle",
			text: "你好 @Bot 課程查詢",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  3,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "你好 課程查詢",
		},
		{
			name: "only bot mention",
			text: "@Bot",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  0,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "",
		},
		{
			name: "bot mention with extra spaces",
			text: "  @Bot   hello  ",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  2,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeBotMentions(tt.text, tt.mention)
			if result != tt.expected {
				t.Errorf("removeBotMentions() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestRemoveBotMentions_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		mention  *webhook.Mention
		expected string
	}{
		{
			name: "index out of bounds (negative)",
			text: "hello",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  -1,
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "lo", // negative index is clamped to 0, so removes first 4 chars
		},
		{
			name: "length exceeds text length",
			text: "@Bot",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  0,
						Length: 100, // Much longer than text
						IsSelf: true,
					},
				},
			},
			expected: "",
		},
		{
			name: "index at exact end of string",
			text: "hello",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  5, // At end, nothing to remove
						Length: 4,
						IsSelf: true,
					},
				},
			},
			expected: "hello",
		},
		{
			name: "zero length mention",
			text: "hello @Bot world",
			mention: &webhook.Mention{
				Mentionees: []webhook.MentioneeInterface{
					webhook.UserMentionee{
						Index:  6,
						Length: 0,
						IsSelf: true,
					},
				},
			},
			expected: "hello @Bot world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeBotMentions(tt.text, tt.mention)
			if result != tt.expected {
				t.Errorf("removeBotMentions() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

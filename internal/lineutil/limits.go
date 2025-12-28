package lineutil

// LINE API Character Limits (Rune count)
// References: https://developers.line.biz/en/reference/messaging-api/
const (
	MaxTextMessageLength = 5000 // Text message max content length
	MaxAltTextLength     = 400  // Template/Flex message alt text length
	MaxPostbackData      = 300  // Postback action data length

	// Template Message Limits
	MaxTemplateTitleLength   = 40  // Buttons/Carousel template title
	MaxTemplateTextNoImage   = 160 // Buttons template text without image
	MaxTemplateTextWithImage = 60  // Buttons template text with image
	MaxCarouselTemplateText  = 60  // Carousel template text (always with/without image)
	MaxCarouselColumnCount   = 10  // Max columns in a carousel
	MaxTemplateActionCount   = 4   // Max actions per template column

	// Flex Message Limits
	MaxFlexCarouselBubbleCount = 12 // Max bubbles in a Flex carousel

	// Quick Reply Limits
	MaxQuickReplyItemCount = 13 // Max items in a quick reply
	MaxQuickReplyLabel     = 20 // Max label length for quick reply item
)

// Safe Buffer Limits (Application-defined for safety or UX)
const (
	// TextListSafeBuffer is used when building long text lists to leave room for headers/footers
	// and avoid hitting the 5000 char limit precisely.
	TextListSafeBuffer = 4900

	// CommonLabelLimit is a general purpose limit for labels/titles in UI components
	CommonLabelLimit = 40
)

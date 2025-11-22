# lineutil

LINE 訊息建構工具，基於 LINE Bot SDK v8。

## 主要功能

- 文字訊息：`NewTextMessage()`
- 輪播訊息：`NewCarouselTemplate()`
- 按鈕訊息：`NewButtonsTemplate()`
- 錯誤模板：`ErrorMessage()`, `ServiceUnavailableMessage()`, `NoResultsMessage()`

## LINE API 限制

- 每次最多 **5 則**訊息
- Carousel 標題最多 **40 字**
- Buttons 動作最多 **4 個**

詳細範例請參考 `example_test.go`。

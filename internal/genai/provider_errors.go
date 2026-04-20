package genai

import (
	"errors"
	"net/http"

	"github.com/openai/openai-go/v3"
	"google.golang.org/api/googleapi"
)

func normalizeProviderError(err error, provider Provider) error {
	if err == nil {
		return nil
	}

	var llmErr *LLMError
	if errors.As(err, &llmErr) {
		return err
	}

	var openaiErr *openai.Error
	if errors.As(err, &openaiErr) {
		return &LLMError{
			Err:        err,
			StatusCode: openaiErr.StatusCode,
			Provider:   provider,
			Headers:    cloneHeaders(openaiErr.Response),
		}
	}

	var googleErr *googleapi.Error
	if errors.As(err, &googleErr) {
		return &LLMError{
			Err:        err,
			StatusCode: googleErr.Code,
			Provider:   provider,
			Headers:    googleErr.Header.Clone(),
		}
	}

	if statusCode := inferStatusCodeFromError(err); statusCode > 0 {
		return &LLMError{
			Err:        err,
			StatusCode: statusCode,
			Provider:   provider,
		}
	}

	return err
}

func cloneHeaders(resp *http.Response) http.Header {
	if resp == nil || resp.Header == nil {
		return nil
	}
	return resp.Header.Clone()
}

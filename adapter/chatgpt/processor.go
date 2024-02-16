package chatgpt

import (
	"chat/globals"
	"chat/utils"
	"errors"
	"fmt"
	"regexp"
)

func formatMessages(props *ChatProps) interface{} {
	if globals.IsOpenAIVisionModels(props.Model) {
		return utils.Each[globals.Message, Message](props.Message, func(message globals.Message) Message {
			if message.Role == globals.User {
				content, urls := utils.ExtractImages(message.Content, true)
				images := utils.EachNotNil[string, MessageContent](urls, func(url string) *MessageContent {
					obj, err := utils.NewImage(url)
					props.Buffer.AddImage(obj)
					if err != nil {
						globals.Info(fmt.Sprintf("cannot process image: %s (source: %s)", err.Error(), utils.Extract(url, 24, "...")))
					}

					return &MessageContent{
						Type: "image_url",
						ImageUrl: &ImageUrl{
							Url: url,
						},
					}
				})

				return Message{
					Role: message.Role,
					Content: utils.Prepend(images, MessageContent{
						Type: "text",
						Text: &content,
					}),
					Name:         message.Name,
					FunctionCall: message.FunctionCall,
					ToolCalls:    message.ToolCalls,
					ToolCallId:   message.ToolCallId,
				}
			}

			return Message{
				Role: message.Role,
				Content: MessageContents{
					MessageContent{
						Type: "text",
						Text: &message.Content,
					},
				},
				Name:         message.Name,
				FunctionCall: message.FunctionCall,
				ToolCalls:    message.ToolCalls,
				ToolCallId:   message.ToolCallId,
			}
		})
	}

	return props.Message
}

func processChatResponse(data string) *ChatStreamResponse {
	return utils.UnmarshalForm[ChatStreamResponse](data)
}

func processCompletionResponse(data string) *CompletionResponse {
	return utils.UnmarshalForm[CompletionResponse](data)
}

func processChatErrorResponse(data string) *ChatStreamErrorResponse {
	return utils.UnmarshalForm[ChatStreamErrorResponse](data)
}

func getChoices(buffer utils.Buffer, form *ChatStreamResponse) string {
	if len(form.Choices) == 0 {
		return ""
	}

	choice := form.Choices[0].Delta

	buffer.AddToolCalls(choice.ToolCalls)
	buffer.SetFunctionCall(choice.FunctionCall)
	return choice.Content
}

func getCompletionChoices(form *CompletionResponse) string {
	if len(form.Choices) == 0 {
		return ""
	}

	return form.Choices[0].Text
}

func getRobustnessResult(chunk string) string {
	exp := `\"content\":\"(.*?)\"`
	compile, err := regexp.Compile(exp)
	if err != nil {
		return ""
	}

	matches := compile.FindStringSubmatch(chunk)
	if len(matches) > 1 {
		return utils.ProcessRobustnessChar(matches[1])
	} else {
		return ""
	}
}

func (c *ChatInstance) ProcessLine(obj utils.Buffer, data string, isCompletionType bool) (string, error) {
	if isCompletionType {
		// legacy support
		if completion := processCompletionResponse(data); completion != nil {
			return getCompletionChoices(completion), nil
		}

		globals.Warn(fmt.Sprintf("chatgpt error: cannot parse completion response: %s", data))
		return "", errors.New("parser error: cannot parse completion response")
	}

	if form := processChatResponse(data); form != nil {
		return getChoices(obj, form), nil
	}

	if form := processChatErrorResponse(data); form != nil {
		return "", errors.New(fmt.Sprintf("chatgpt error: %s (type: %s)", form.Error.Message, form.Error.Type))
	}

	globals.Warn(fmt.Sprintf("chatgpt error: cannot parse chat completion response: %s", data))
	return "", errors.New("parser error: cannot parse chat completion response")
}

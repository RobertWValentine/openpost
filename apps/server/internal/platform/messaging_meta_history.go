package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

type metaMessageView struct {
	ID          string `json:"id"`
	Message     string `json:"message"`
	CreatedTime string `json:"created_time"`
	From        struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"from"`
	Attachments struct {
		Data []struct {
			MimeType string `json:"mime_type"`
			Name     string `json:"name"`
			Image    struct {
				Src string `json:"src"`
			} `json:"image_data"`
			Video struct {
				URL string `json:"url"`
			} `json:"video_data"`
			FileURL string `json:"file_url"`
		} `json:"data"`
	} `json:"attachments"`
}

func (m metaMessageView) provider(accountID string) ProviderMessage {
	// Graph returns offsets without a colon, such as 2026-10-26T21:01:00+0000.
	at := nativePostTime(m.CreatedTime)
	direction := "inbound"
	if m.From.ID == accountID {
		direction = "outbound"
	}
	result := ProviderMessage{ID: m.ID, Direction: direction, AuthorRemoteID: m.From.ID, Body: m.Message, RemoteCreatedAt: at}
	for _, attachment := range m.Attachments.Data {
		remote := firstNonEmptyString(attachment.Image.Src, attachment.Video.URL, attachment.FileURL)
		if remote != "" {
			result.Attachments = append(result.Attachments, MessageAttachment{Type: metaAttachmentType(attachment.MimeType), URL: remote, Name: attachment.Name, MimeType: attachment.MimeType})
		}
	}
	return result
}

func (f *FacebookAdapter) FetchMessageHistory(ctx context.Context, token string, input FetchMessageHistoryRequest) (FetchMessageHistoryResult, error) {
	return (metaMessagingAdapter{graphURL: f.graphURL}).history(ctx, token, input)
}
func (i *InstagramAdapter) FetchMessageHistory(ctx context.Context, token string, input FetchMessageHistoryRequest) (FetchMessageHistoryResult, error) {
	return i.messagingAdapter().history(ctx, token, input)
}

func (m metaMessagingAdapter) history(ctx context.Context, token string, input FetchMessageHistoryRequest) (FetchMessageHistoryResult, error) {
	limit := input.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	query := url.Values{"limit": {fmt.Sprint(limit)}, "fields": {"id,from,message,created_time,attachments"}}
	if input.Cursor != "" {
		query.Set("after", input.Cursor)
	}
	body, err := DoRequest(ctx, http.MethodGet, m.graphURL(url.PathEscape(input.ConversationID)+"/messages")+"?"+query.Encode(), nil, map[string]string{headerAuthorization: bearerPrefix + token})
	if err != nil {
		return FetchMessageHistoryResult{}, err
	}
	var response struct {
		Data   []metaMessageView `json:"data"`
		Paging struct {
			Next    string `json:"next"`
			Cursors struct {
				After string `json:"after"`
			} `json:"cursors"`
		} `json:"paging"`
	}
	if err = json.Unmarshal(body, &response); err != nil {
		return FetchMessageHistoryResult{}, fmt.Errorf("decoding Meta history: %w", err)
	}
	result := FetchMessageHistoryResult{}
	if response.Paging.Next != "" {
		result.NextCursor = response.Paging.Cursors.After
	}
	for _, message := range response.Data {
		if message.ID != "" {
			result.Messages = append(result.Messages, message.provider(input.AccountID))
		}
	}
	sort.SliceStable(result.Messages, func(i, j int) bool {
		return result.Messages[i].RemoteCreatedAt.Before(result.Messages[j].RemoteCreatedAt)
	})
	return result, nil
}

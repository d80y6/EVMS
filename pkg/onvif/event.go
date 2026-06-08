package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

type PullPointSubscription struct {
	Address       string
	CurrentTime   time.Time
	TerminationTime time.Time
}

type ONVIFEvent struct {
	Topic     string
	Timestamp time.Time
	Source    map[string]string
	Data      map[string]interface{}
}

func CreatePullPointSubscription(ctx context.Context, client *SOAPClient, eventURL string, initialTerminationTime time.Duration) (*PullPointSubscription, error) {
	itt := fmt.Sprintf("PT%ds", int(initialTerminationTime.Seconds()))

	body := fmt.Sprintf(`<wsnt:CreatePullPointSubscription xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
    <wsnt:InitialTerminationTime>%s</wsnt:InitialTerminationTime>
  </wsnt:CreatePullPointSubscription>`, itt)

	data, err := client.Do(ctx, eventURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("CreatePullPointSubscription failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Address         string `xml:"SubscriptionReference>Address"`
				CurrentTime     string `xml:"CurrentTime"`
				TerminationTime string `xml:"TerminationTime"`
			} `xml:"CreatePullPointSubscriptionResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse subscription response: %w", err)
	}

	sub := &PullPointSubscription{
		Address: resp.Body.Response.Address,
	}

	if t, err := time.Parse(time.RFC3339, resp.Body.Response.CurrentTime); err == nil {
		sub.CurrentTime = t
	}
	if t, err := time.Parse(time.RFC3339, resp.Body.Response.TerminationTime); err == nil {
		sub.TerminationTime = t
	}

	return sub, nil
}

func PullMessages(ctx context.Context, client *SOAPClient, pullPointURL string, maxMessages int, timeout time.Duration) ([]ONVIFEvent, error) {
	timeoutStr := fmt.Sprintf("PT%ds", int(timeout.Seconds()))
	if maxMessages <= 0 {
		maxMessages = 10
	}

	body := fmt.Sprintf(`<wsnt:PullMessages xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
    <wsnt:MaxNumberOfMessages>%d</wsnt:MaxNumberOfMessages>
    <wsnt:Timeout>%s</wsnt:Timeout>
  </wsnt:PullMessages>`, maxMessages, timeoutStr)

	data, err := client.Do(ctx, pullPointURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("PullMessages failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				NotificationMessage []struct {
					Topic struct {
						Text string `xml:",chardata"`
					} `xml:"Topic"`
					Message struct {
						Data string `xml:",innerxml"`
					} `xml:"Message"`
				} `xml:"NotificationMessage"`
			} `xml:"PullMessagesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		notificationCount := 0
		var topics []string
		decoder := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			if start, ok := token.(xml.StartElement); ok {
				if start.Name.Local == "NotificationMessage" {
					notificationCount++
				}
				if start.Name.Local == "Topic" {
					if charToken, err := decoder.Token(); err == nil {
						if chars, ok := charToken.(xml.CharData); ok {
							if topic := strings.TrimSpace(string(chars)); topic != "" {
								topics = append(topics, topic)
							}
						}
					}
				}
			}
		}

		events := make([]ONVIFEvent, 0, len(topics))
		for _, topic := range topics {
			events = append(events, ONVIFEvent{
				Topic:     topic,
				Timestamp: time.Now().UTC(),
			})
		}

		if len(events) > 0 {
			return events, nil
		}
		return nil, fmt.Errorf("parsed %d notifications, extracting topics via fallback", notificationCount)
	}

	events := make([]ONVIFEvent, 0, len(resp.Body.Response.NotificationMessage))
	for _, msg := range resp.Body.Response.NotificationMessage {
		event := ONVIFEvent{
			Topic:     msg.Topic.Text,
			Timestamp: time.Now().UTC(),
			Data:      make(map[string]interface{}),
		}
		events = append(events, event)
	}

	return events, nil
}

func RenewPullPointSubscription(ctx context.Context, client *SOAPClient, pullPointURL string, terminationTime time.Duration) error {
	itt := fmt.Sprintf("PT%ds", int(terminationTime.Seconds()))

	body := fmt.Sprintf(`<wsnt:Renew xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
    <wsnt:TerminationTime>%s</wsnt:TerminationTime>
  </wsnt:Renew>`, itt)

	_, err := client.Do(ctx, pullPointURL, "", body)
	return err
}

func UnsubscribePullPoint(ctx context.Context, client *SOAPClient, pullPointURL string) error {
	body := `<wsnt:Unsubscribe xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"/>`

	_, err := client.Do(ctx, pullPointURL, "", body)
	return err
}

func ClassifyEventTopic(topic string) string {
	topicLower := strings.ToLower(topic)

	switch {
	case strings.Contains(topicLower, "cellmotion"), strings.Contains(topicLower, "cell_motion"):
		return "cell_motion"
	case strings.Contains(topicLower, "motion"):
		return "motion"
	case strings.Contains(topicLower, "tamper"), strings.Contains(topicLower, "tampering"):
		return "tamper"
	case strings.Contains(topicLower, "alarm"):
		return "alarm"
	case strings.Contains(topicLower, "relay"):
		return "relay"
	case strings.Contains(topicLower, "digital"), strings.Contains(topicLower, "input"):
		return "digital_input"
	case strings.Contains(topicLower, "acoustic"), strings.Contains(topicLower, "audio"):
		return "audio"
	case strings.Contains(topicLower, "fielddetection"), strings.Contains(topicLower, "field_detection"):
		return "field_detection"
	case strings.Contains(topicLower, "linedetection"), strings.Contains(topicLower, "line_detection"), strings.Contains(topicLower, "linecross"):
		return "line_crossing"
	case strings.Contains(topicLower, "count"):
		return "people_count"
	case strings.Contains(topicLower, "face"):
		return "face_detection"
	case strings.Contains(topicLower, "parking"):
		return "parking"
	case strings.Contains(topicLower, "loiter"):
		return "loitering"
	default:
		return ""
	}
}

func GetEventProperties(ctx context.Context, client *SOAPClient, eventURL string) ([]string, error) {
	body := `<wsnt:GetEventProperties xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"/>`

	data, err := client.Do(ctx, eventURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var topics []string
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "Topic" {
			if charToken, err := decoder.Token(); err == nil {
				if chars, ok := charToken.(xml.CharData); ok {
					if topic := strings.TrimSpace(string(chars)); topic != "" {
						topics = append(topics, topic)
					}
				}
			}
		}
	}

	return topics, nil
}

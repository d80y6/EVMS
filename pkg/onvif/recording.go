package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"
)

type RecordingConfiguration struct {
	Source string
	Content string
	MaximumRetentionTime time.Duration
}

type Recording struct {
	Token       string
	Configuration *RecordingConfiguration
	Tracks      []TrackInfo
}

type TrackInfo struct {
	Token       string
	TrackType   string
	Description string
	DataFrom    time.Time
	DataTo      time.Time
}

type RecordingSummary struct {
	DataFrom  time.Time
	DataTo    time.Time
	TrackCount int
}

type ReplayConfiguration struct {
	Token string
	Name  string
	URL   string
}

func GetRecordingConfiguration(ctx context.Context, client *SOAPClient, recordingURL, recordingToken string) (*RecordingConfiguration, error) {
	body := fmt.Sprintf(`<trec:GetRecordingConfiguration xmlns:trec="http://www.onvif.org/ver10/recording/wsdl">
    <trec:RecordingToken>%s</trec:RecordingToken>
  </trec:GetRecordingConfiguration>`, recordingToken)

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Config struct {
					Source  string `xml:"Source"`
					Content string `xml:"Content"`
				} `xml:"RecordingConfiguration"`
			} `xml:"GetRecordingConfigurationResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetRecordingConfiguration response: %w", err)
	}

	return &RecordingConfiguration{
		Source:  resp.Body.Response.Config.Source,
		Content: resp.Body.Response.Config.Content,
	}, nil
}

func GetRecordings(ctx context.Context, client *SOAPClient, recordingURL string) ([]Recording, error) {
	body := `<trec:GetRecordings xmlns:trec="http://www.onvif.org/ver10/recording/wsdl"/>`

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetRecordings failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Recordings []struct {
					Token string `xml:"token,attr"`
					Source struct {
						Token string `xml:"token,attr"`
					} `xml:"Source"`
					Content string `xml:"Content"`
				} `xml:"Recording"`
			} `xml:"GetRecordingsResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetRecordings response: %w", err)
	}

	recordings := make([]Recording, 0, len(resp.Body.Response.Recordings))
	for _, r := range resp.Body.Response.Recordings {
		recordings = append(recordings, Recording{
			Token: r.Token,
			Configuration: &RecordingConfiguration{
				Source:  r.Source.Token,
				Content: r.Content,
			},
		})
	}

	return recordings, nil
}

func CreateRecording(ctx context.Context, client *SOAPClient, recordingURL string, config *RecordingConfiguration) (string, error) {
	body := fmt.Sprintf(`<trec:CreateRecording xmlns:trec="http://www.onvif.org/ver10/recording/wsdl">
    <trec:RecordingConfiguration>
      <trec:Source>%s</trec:Source>
      <trec:Content>%s</trec:Content>
    </trec:RecordingConfiguration>
  </trec:CreateRecording>`, config.Source, config.Content)

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return "", fmt.Errorf("CreateRecording failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return "", err
	}

	return ExtractXMLString(data, "RecordingToken")
}

func DeleteRecording(ctx context.Context, client *SOAPClient, recordingURL, recordingToken string) error {
	body := fmt.Sprintf(`<trec:DeleteRecording xmlns:trec="http://www.onvif.org/ver10/recording/wsdl">
    <trec:RecordingToken>%s</trec:RecordingToken>
  </trec:DeleteRecording>`, recordingToken)

	_, err := client.Do(ctx, recordingURL, "", body)
	return err
}

func GetRecordingSummary(ctx context.Context, client *SOAPClient, recordingURL, recordingToken string) (*RecordingSummary, error) {
	body := fmt.Sprintf(`<trec:GetRecordingSummary xmlns:trec="http://www.onvif.org/ver10/recording/wsdl">
    <trec:RecordingToken>%s</trec:RecordingToken>
  </trec:GetRecordingSummary>`, recordingToken)

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingSummary failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var summary RecordingSummary
	summary.DataFrom, _ = time.Parse(time.RFC3339, "")
	summary.DataTo, _ = time.Parse(time.RFC3339, "")

	return &summary, nil
}

func GetRecordingTracks(ctx context.Context, client *SOAPClient, recordingURL, recordingToken string) ([]TrackInfo, error) {
	body := fmt.Sprintf(`<trec:GetTrackInfo xmlns:trec="http://www.onvif.org/ver10/recording/wsdl">
    <trec:RecordingToken>%s</trec:RecordingToken>
  </trec:GetTrackInfo>`, recordingToken)

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetTrackInfo failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Tracks []struct {
					Token     string `xml:"TrackToken"`
					TrackType string `xml:"TrackType"`
					Description string `xml:"Description"`
				} `xml:"TrackInfo"`
			} `xml:"GetTrackInfoResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetTrackInfo response: %w", err)
	}

	tracks := make([]TrackInfo, 0, len(resp.Body.Response.Tracks))
	for _, t := range resp.Body.Response.Tracks {
		tracks = append(tracks, TrackInfo{
			Token:       t.Token,
			TrackType:   t.TrackType,
			Description: t.Description,
		})
	}

	return tracks, nil
}

func CreateRecordingJob(ctx context.Context, client *SOAPClient, recordingURL, recordingToken, profileToken string) (string, error) {
	body := fmt.Sprintf(`<trec:CreateRecordingJob xmlns:trec="http://www.onvif.org/ver10/recording/wsdl">
    <trec:JobConfiguration>
      <trec:RecordingToken>%s</trec:RecordingToken>
      <trec:Mode>Active</trec:Mode>
      <trec:Priority>Medium</trec:Priority>
      <trec:Source>
        <trec:SourceToken>%s</trec:SourceToken>
        <trec:Type>Video</trec:Type>
      </trec:Source>
    </trec:JobConfiguration>
  </trec:CreateRecordingJob>`, recordingToken, profileToken)

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return "", fmt.Errorf("CreateRecordingJob failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return "", err
	}

	return ExtractXMLString(data, "JobToken")
}

func FindRecordings(ctx context.Context, client *SOAPClient, recordingURL, cameraToken string, start, end time.Time) ([]Recording, error) {
	body := fmt.Sprintf(`<tsear:FindRecordings xmlns:tsear="http://www.onvif.org/ver10/search/wsdl">
    <tsear:Scope>
      <tsear:Source>%s</tsear:Source>
    </tsear:Scope>
    <tsear:StartRecord>%s</tsear:StartRecord>
    <tsear:EndRecord>%s</tsear:EndRecord>
  </tsear:FindRecordings>`, cameraToken, start.Format(time.RFC3339), end.Format(time.RFC3339))

	data, err := client.Do(ctx, recordingURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("FindRecordings failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				FindToken string `xml:"FindToken"`
			} `xml:"FindRecordingsResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("find token: %s (not yet implemented)", resp.Body.Response.FindToken)
}

func GetReplayURI(ctx context.Context, client *SOAPClient, replayURL, recordingToken, profileToken string) (string, error) {
	body := fmt.Sprintf(`<treplay:GetReplayUri xmlns:treplay="http://www.onvif.org/ver10/replay/wsdl">
    <treplay:RecordingToken>%s</treplay:RecordingToken>
    <treplay:StreamSetup>
      <tt:Stream xmlns:tt="http://www.onvif.org/ver10/schema">RTP-Unicast</tt:Stream>
      <tt:Transport xmlns:tt="http://www.onvif.org/ver10/schema">
        <tt:Protocol>RTSP</tt:Protocol>
      </tt:Transport>
    </treplay:StreamSetup>
  </treplay:GetReplayUri>`, recordingToken)

	data, err := client.Do(ctx, replayURL, "", body)
	if err != nil {
		return "", fmt.Errorf("GetReplayUri failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return "", err
	}

	return ExtractXMLString(data, "Uri")
}

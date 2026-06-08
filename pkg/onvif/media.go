package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Profile struct {
	Token   string
	Name    string
	Fixed   bool
	VideoSource *struct {
		Token     string
		ResolutionWidth  int
		ResolutionHeight int
	}
	AudioSource *struct {
		Token string
	}
	PTZConfiguration *struct {
		Token string
		Name  string
	}
	VideoEncoderConfiguration *VideoEncoderConfig
	AudioEncoderConfiguration *struct {
		Token string
		Name  string
	}
}

type VideoEncoderConfig struct {
	Token     string
	Name      string
	Encoding  string
	Width     int
	Height    int
	Quality   int
	FrameRate float64
	Bitrate   int
	GovLength int
}

type StreamURI struct {
	URI string
	InvalidAfterConnect bool
	InvalidAfterReboot  bool
	Timeout             string
}

type VideoSource struct {
	Token           string
	ResolutionWidth  int
	ResolutionHeight int
	Framerate        float64
}

type AudioSource struct {
	Token    string
	Channels int
}

type VideoEncoderConfigurationOptions struct {
	QualityRange struct {
		Min int
		Max int
	}
	JPEG *struct {
		ResolutionsAvailable []struct {
			Width  int
			Height int
		}
		FrameRateRange struct {
			Min float64
			Max float64
		}
		BitrateRange struct {
			Min int
			Max int
		}
	}
	H264 *struct {
		ResolutionsAvailable []struct {
			Width  int
			Height int
		}
		FrameRateRange struct {
			Min float64
			Max float64
		}
		BitrateRange struct {
			Min int
		}
		GovLengthRange struct {
			Min int
			Max int
		}
	}
	H265 *struct {
		ResolutionsAvailable []struct {
			Width  int
			Height int
		}
	}
}

func GetProfiles(ctx context.Context, client *SOAPClient, mediaURL string) ([]Profile, error) {
	body := `<trt:GetProfiles xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetProfiles failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Profiles []struct {
					Token   string `xml:"token,attr"`
					Name    string `xml:"Name"`
					Fixed   bool   `xml:"fixed,attr"`
					VideoSource *struct {
						Token string `xml:"token,attr"`
					} `xml:"VideoSource"`
					AudioSource *struct {
						Token string `xml:"token,attr"`
					} `xml:"AudioSource"`
					PTZConfiguration *struct {
						Token string `xml:"token,attr"`
						Name  string `xml:"Name"`
					} `xml:"PTZConfiguration"`
					VideoEncoderConfiguration *struct {
						Token    string `xml:"token,attr"`
						Name     string `xml:"Name"`
						Encoding string `xml:"Encoding"`
						Resolution struct {
							Width  int `xml:"Width"`
							Height int `xml:"Height"`
						} `xml:"Resolution"`
						Quality   int     `xml:"Quality"`
						FrameRate float64 `xml:"FrameRateLimit"`
						Bitrate   int     `xml:"BitrateLimit"`
						GovLength int     `xml:"GovLength"`
					} `xml:"VideoEncoderConfiguration"`
					AudioEncoderConfiguration *struct {
						Token string `xml:"token,attr"`
						Name  string `xml:"Name"`
					} `xml:"AudioEncoderConfiguration"`
				} `xml:"Profiles"`
			} `xml:"GetProfilesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetProfiles response: %w", err)
	}

	profiles := make([]Profile, 0, len(resp.Body.Response.Profiles))
	for _, p := range resp.Body.Response.Profiles {
		profile := Profile{
			Token: p.Token,
			Name:  p.Name,
			Fixed: p.Fixed,
		}

		if p.VideoSource != nil {
			profile.VideoSource = &struct {
				Token string
				ResolutionWidth  int
				ResolutionHeight int
			}{Token: p.VideoSource.Token}
		}

		if p.AudioSource != nil {
			profile.AudioSource = &struct{ Token string }{Token: p.AudioSource.Token}
		}

		if p.PTZConfiguration != nil {
			profile.PTZConfiguration = &struct {
				Token string
				Name  string
			}{Token: p.PTZConfiguration.Token, Name: p.PTZConfiguration.Name}
		}

		if p.VideoEncoderConfiguration != nil {
			profile.VideoEncoderConfiguration = &VideoEncoderConfig{
				Token:     p.VideoEncoderConfiguration.Token,
				Name:      p.VideoEncoderConfiguration.Name,
				Encoding:  p.VideoEncoderConfiguration.Encoding,
				Width:     p.VideoEncoderConfiguration.Resolution.Width,
				Height:    p.VideoEncoderConfiguration.Resolution.Height,
				Quality:   p.VideoEncoderConfiguration.Quality,
				FrameRate: p.VideoEncoderConfiguration.FrameRate,
				Bitrate:   p.VideoEncoderConfiguration.Bitrate,
				GovLength: p.VideoEncoderConfiguration.GovLength,
			}
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

func GetProfile(ctx context.Context, client *SOAPClient, mediaURL, profileToken string) (*Profile, error) {
	body := fmt.Sprintf(`<trt:GetProfile xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
    <trt:ProfileToken>%s</trt:ProfileToken>
  </trt:GetProfile>`, profileToken)

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetProfile failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Profile Profile `xml:"Profile"`
			} `xml:"GetProfileResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetProfile response: %w", err)
	}

	return &resp.Body.Response.Profile, nil
}

func GetStreamURI(ctx context.Context, client *SOAPClient, mediaURL, profileToken, protocol string) (*StreamURI, error) {
	if protocol == "" {
		protocol = "RTSP"
	}
	if protocol != "RTSP" && protocol != "RTMP" {
		protocol = "RTSP"
	}

	body := fmt.Sprintf(`<trt:GetStreamUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
    <trt:StreamSetup>
      <tt:Stream>RTP-Unicast</tt:Stream>
      <tt:Transport>
        <tt:Protocol>%s</tt:Protocol>
      </tt:Transport>
    </trt:StreamSetup>
    <trt:ProfileToken>%s</trt:ProfileToken>
  </trt:GetStreamUri>`, protocol, profileToken)

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetStreamUri failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				MediaUri struct {
					URI                 string `xml:"Uri"`
					InvalidAfterConnect bool   `xml:"InvalidAfterConnect"`
					InvalidAfterReboot  bool   `xml:"InvalidAfterReboot"`
					Timeout             string `xml:"Timeout"`
				} `xml:"MediaUri"`
			} `xml:"GetStreamUriResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetStreamUri response: %w", err)
	}

	uri := resp.Body.Response.MediaUri.URI
	if !strings.HasPrefix(uri, "rtsp://") && !strings.HasPrefix(uri, "rtmp://") {
		parsed, err := url.Parse(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid stream URI: %s", uri)
		}
		uri = parsed.String()
	}

	return &StreamURI{
		URI:                 uri,
		InvalidAfterConnect: resp.Body.Response.MediaUri.InvalidAfterConnect,
		InvalidAfterReboot:  resp.Body.Response.MediaUri.InvalidAfterReboot,
		Timeout:             resp.Body.Response.MediaUri.Timeout,
	}, nil
}

func GetSnapshotURI(ctx context.Context, client *SOAPClient, mediaURL, profileToken string) (string, error) {
	body := fmt.Sprintf(`<trt:GetSnapshotUri xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
    <trt:ProfileToken>%s</trt:ProfileToken>
  </trt:GetSnapshotUri>`, profileToken)

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return "", fmt.Errorf("GetSnapshotUri failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return "", err
	}

	var resp struct {
		Body struct {
			Response struct {
				Uri struct {
					Uri string `xml:"Uri"`
				} `xml:"Uri"`
			} `xml:"GetSnapshotUriResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		uri, err := ExtractXMLString(data, "Uri")
		if err != nil {
			return "", fmt.Errorf("failed to parse GetSnapshotUri response: %w", err)
		}
		return uri, nil
	}

	return resp.Body.Response.Uri.Uri, nil
}

func GetVideoSources(ctx context.Context, client *SOAPClient, mediaURL string) ([]VideoSource, error) {
	body := `<trt:GetVideoSources xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Sources []struct {
					Token      string `xml:"token,attr"`
					Resolution struct {
						Width  int `xml:"Width"`
						Height int `xml:"Height"`
					} `xml:"Resolution"`
					Framerate float64 `xml:"Framerate"`
				} `xml:"VideoSources"`
			} `xml:"GetVideoSourcesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	sources := make([]VideoSource, 0, len(resp.Body.Response.Sources))
	for _, s := range resp.Body.Response.Sources {
		sources = append(sources, VideoSource{
			Token:           s.Token,
			ResolutionWidth:  s.Resolution.Width,
			ResolutionHeight: s.Resolution.Height,
			Framerate:        s.Framerate,
		})
	}

	return sources, nil
}

func GetAudioSources(ctx context.Context, client *SOAPClient, mediaURL string) ([]AudioSource, error) {
	body := `<trt:GetAudioSources xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Sources []struct {
					Token    string `xml:"token,attr"`
					Channels int    `xml:"Channels"`
				} `xml:"AudioSources"`
			} `xml:"GetAudioSourcesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	sources := make([]AudioSource, 0, len(resp.Body.Response.Sources))
	for _, s := range resp.Body.Response.Sources {
		sources = append(sources, AudioSource{
			Token:    s.Token,
			Channels: s.Channels,
		})
	}

	return sources, nil
}

func GetVideoEncoderConfigurationOptions(ctx context.Context, client *SOAPClient, mediaURL, profileToken string) (*VideoEncoderConfigurationOptions, error) {
	body := `<trt:GetVideoEncoderConfigurationOptions xmlns:trt="http://www.onvif.org/ver10/media/wsdl"/>`

	if profileToken != "" {
		body = fmt.Sprintf(`<trt:GetVideoEncoderConfigurationOptions xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
      <trt:ProfileToken>%s</trt:ProfileToken>
    </trt:GetVideoEncoderConfigurationOptions>`, profileToken)
	}

	data, err := client.Do(ctx, mediaURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var opts VideoEncoderConfigurationOptions
	if err := xml.Unmarshal(data, &opts); err != nil {
		return nil, err
	}

	return &opts, nil
}

func SetVideoEncoderConfiguration(ctx context.Context, client *SOAPClient, mediaURL string, config *VideoEncoderConfig) error {
	body := fmt.Sprintf(`<trt:SetVideoEncoderConfiguration xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
    <trt:Configuration>
      <tt:Token>%s</tt:Token>
      <tt:Name>%s</tt:Name>
      <tt:Encoding>%s</tt:Encoding>
      <tt:Resolution>
        <tt:Width>%d</tt:Width>
        <tt:Height>%d</tt:Height>
      </tt:Resolution>
      <tt:Quality>%d</tt:Quality>
      <tt:FrameRateLimit>%s</tt:FrameRateLimit>
      <tt:BitrateLimit>%d</tt:BitrateLimit>
    </trt:Configuration>
    <trt:ForcePersistence>true</trt:ForcePersistence>
  </trt:SetVideoEncoderConfiguration>`,
		config.Token, config.Name, config.Encoding,
		config.Width, config.Height, config.Quality,
		strconv.FormatFloat(config.FrameRate, 'f', -1, 64),
		config.Bitrate)

	_, err := client.Do(ctx, mediaURL, "", body)
	return err
}

func GetStreamURIForProfileToken(ctx context.Context, client *SOAPClient, baseURL, token string) (string, error) {
	mediaURL := BuildMediaURL(baseURL)
	uri, err := GetStreamURI(ctx, client, mediaURL, token, "RTSP")
	if err != nil {
		uri, err = GetStreamURI(ctx, client, mediaURL, token, "RTMP")
		if err != nil {
			return "", err
		}
	}
	return uri.URI, nil
}

func FindMainProfile(profiles []Profile) *Profile {
	for _, p := range profiles {
		if p.VideoEncoderConfiguration != nil && p.VideoEncoderConfiguration.Encoding == "H264" && p.VideoEncoderConfiguration.Width >= 1920 {
			return &p
		}
	}
	for _, p := range profiles {
		if p.VideoEncoderConfiguration != nil {
			return &p
		}
	}
	if len(profiles) > 0 {
		return &profiles[0]
	}
	return nil
}

func FindSubProfile(profiles []Profile, mainToken string) *Profile {
	for _, p := range profiles {
		if p.Token != mainToken && p.VideoEncoderConfiguration != nil {
			return &p
		}
	}
	return nil
}

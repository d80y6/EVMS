package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
)

type ImagingSettings struct {
	Brightness       *float64
	ColorSaturation  *float64
	Contrast         *float64
	Sharpness        *float64
	Exposure         *ExposureSettings
	Focus            *FocusSettings
	WhiteBalance     *WhiteBalanceSettings
	WideDynamicRange *WDRSettings
	BacklightCompensation *BacklightSettings
	IrCutFilter      string
}

type ExposureSettings struct {
	Mode        string
	Priority    string
	MinExposure float64
	MaxExposure float64
	MinGain     float64
	MaxGain     float64
	MinIris     float64
	MaxIris     float64
	ExposureTime float64
	Gain        float64
	Iris        float64
}

type FocusSettings struct {
	Mode           string
	DefaultSpeed   float64
	NearLimit      float64
	FarLimit       float64
}

type WhiteBalanceSettings struct {
	Mode    string
	CrGain  float64
	CbGain  float64
}

type WDRSettings struct {
	Mode  string
	Level float64
}

type BacklightSettings struct {
	Mode  string
	Level float64
}

type ImagingStatus struct {
	FocusState   string
	ExposureMode string
}

func GetImagingSettings(ctx context.Context, client *SOAPClient, imagingURL, videoSourceToken string) (*ImagingSettings, error) {
	body := fmt.Sprintf(`<tim:GetImagingSettings xmlns:tim="http://www.onvif.org/ver20/imaging/wsdl">
    <tim:VideoSourceToken>%s</tim:VideoSourceToken>
  </tim:GetImagingSettings>`, videoSourceToken)

	data, err := client.Do(ctx, imagingURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetImagingSettings failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Settings struct {
					Brightness       *float64 `xml:"Brightness"`
					ColorSaturation  *float64 `xml:"ColorSaturation"`
					Contrast         *float64 `xml:"Contrast"`
					Sharpness        *float64 `xml:"Sharpness"`
					Exposure         *struct {
						Mode        string  `xml:"Mode"`
						Priority    string  `xml:"Priority"`
						MinExposure float64 `xml:"MinExposureTime"`
						MaxExposure float64 `xml:"MaxExposureTime"`
						MinGain     float64 `xml:"MinGain"`
						MaxGain     float64 `xml:"MaxGain"`
						MinIris     float64 `xml:"MinIris"`
						MaxIris     float64 `xml:"MaxIris"`
						ExposureTime float64 `xml:"ExposureTime"`
						Gain        float64 `xml:"Gain"`
						Iris        float64 `xml:"Iris"`
					} `xml:"Exposure"`
					Focus *struct {
						Mode         string  `xml:"Mode"`
						DefaultSpeed float64 `xml:"DefaultSpeed"`
						NearLimit    float64 `xml:"NearLimit"`
						FarLimit     float64 `xml:"FarLimit"`
					} `xml:"Focus"`
					WhiteBalance *struct {
						Mode   string  `xml:"Mode"`
						CrGain float64 `xml:"CrGain"`
						CbGain float64 `xml:"CbGain"`
					} `xml:"WhiteBalance"`
					WideDynamicRange *struct {
						Mode  string  `xml:"Mode"`
						Level float64 `xml:"Level"`
					} `xml:"WideDynamicRange"`
					BacklightCompensation *struct {
						Mode  string  `xml:"Mode"`
						Level float64 `xml:"Level"`
					} `xml:"BacklightCompensation"`
					IrCutFilter string `xml:"IrCutFilter"`
				} `xml:"ImagingSettings"`
			} `xml:"GetImagingSettingsResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetImagingSettings response: %w", err)
	}

	settings := &ImagingSettings{
		Brightness:      resp.Body.Response.Settings.Brightness,
		ColorSaturation: resp.Body.Response.Settings.ColorSaturation,
		Contrast:        resp.Body.Response.Settings.Contrast,
		Sharpness:       resp.Body.Response.Settings.Sharpness,
		IrCutFilter:     resp.Body.Response.Settings.IrCutFilter,
	}

	if exp := resp.Body.Response.Settings.Exposure; exp != nil {
		settings.Exposure = &ExposureSettings{
			Mode:         exp.Mode,
			Priority:     exp.Priority,
			MinExposure:  exp.MinExposure,
			MaxExposure:  exp.MaxExposure,
			MinGain:      exp.MinGain,
			MaxGain:      exp.MaxGain,
			MinIris:      exp.MinIris,
			MaxIris:      exp.MaxIris,
			ExposureTime: exp.ExposureTime,
			Gain:         exp.Gain,
			Iris:         exp.Iris,
		}
	}

	if focus := resp.Body.Response.Settings.Focus; focus != nil {
		settings.Focus = &FocusSettings{
			Mode:         focus.Mode,
			DefaultSpeed: focus.DefaultSpeed,
			NearLimit:    focus.NearLimit,
			FarLimit:     focus.FarLimit,
		}
	}

	if wb := resp.Body.Response.Settings.WhiteBalance; wb != nil {
		settings.WhiteBalance = &WhiteBalanceSettings{
			Mode:   wb.Mode,
			CrGain: wb.CrGain,
			CbGain: wb.CbGain,
		}
	}

	if wdr := resp.Body.Response.Settings.WideDynamicRange; wdr != nil {
		settings.WideDynamicRange = &WDRSettings{
			Mode:  wdr.Mode,
			Level: wdr.Level,
		}
	}

	if blc := resp.Body.Response.Settings.BacklightCompensation; blc != nil {
		settings.BacklightCompensation = &BacklightSettings{
			Mode:  blc.Mode,
			Level: blc.Level,
		}
	}

	return settings, nil
}

func SetImagingSettings(ctx context.Context, client *SOAPClient, imagingURL, videoSourceToken string, settings *ImagingSettings) error {
	body := fmt.Sprintf(`<tim:SetImagingSettings xmlns:tim="http://www.onvif.org/ver20/imaging/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
    <tim:VideoSourceToken>%s</tim:VideoSourceToken>
    <tim:ImagingSettings>`, videoSourceToken)

	if settings.Brightness != nil {
		body += fmt.Sprintf(`<tt:Brightness>%f</tt:Brightness>`, *settings.Brightness)
	}
	if settings.ColorSaturation != nil {
		body += fmt.Sprintf(`<tt:ColorSaturation>%f</tt:ColorSaturation>`, *settings.ColorSaturation)
	}
	if settings.Contrast != nil {
		body += fmt.Sprintf(`<tt:Contrast>%f</tt:Contrast>`, *settings.Contrast)
	}
	if settings.Sharpness != nil {
		body += fmt.Sprintf(`<tt:Sharpness>%f</tt:Sharpness>`, *settings.Sharpness)
	}
	if settings.IrCutFilter != "" {
		body += fmt.Sprintf(`<tt:IrCutFilter>%s</tt:IrCutFilter>`, settings.IrCutFilter)
	}

	if settings.Exposure != nil {
		body += `<tt:Exposure>`
		if settings.Exposure.Mode != "" {
			body += fmt.Sprintf(`<tt:Mode>%s</tt:Mode>`, settings.Exposure.Mode)
		}
		body += `</tt:Exposure>`
	}

	if settings.WhiteBalance != nil && settings.WhiteBalance.Mode != "" {
		body += fmt.Sprintf(`<tt:WhiteBalance><tt:Mode>%s</tt:Mode></tt:WhiteBalance>`, settings.WhiteBalance.Mode)
	}
	if settings.WideDynamicRange != nil {
		body += fmt.Sprintf(`<tt:WideDynamicRange><tt:Mode>%s</tt:Mode></tt:WideDynamicRange>`, settings.WideDynamicRange.Mode)
	}
	if settings.BacklightCompensation != nil {
		body += fmt.Sprintf(`<tt:BacklightCompensation><tt:Mode>%s</tt:Mode></tt:BacklightCompensation>`, settings.BacklightCompensation.Mode)
	}

	body += `</tim:ImagingSettings>
    <tim:ForcePersistence>true</tim:ForcePersistence>
  </tim:SetImagingSettings>`

	_, err := client.Do(ctx, imagingURL, "", body)
	return err
}

func GetImagingCapabilities(ctx context.Context, client *SOAPClient, deviceURL string) (map[string]interface{}, error) {
	caps, err := GetCapabilities(ctx, client, deviceURL)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"imaging_supported": caps.Imaging,
	}, nil
}

func MoveFocus(ctx context.Context, client *SOAPClient, imagingURL, videoSourceToken string, speed float64) error {
	body := fmt.Sprintf(`<tim:Move xmlns:tim="http://www.onvif.org/ver20/imaging/wsdl">
    <tim:VideoSourceToken>%s</tim:VideoSourceToken>
    <tim:Focus>
      <tim:Continuous>
        <tim:Speed>%f</tim:Speed>
      </tim:Continuous>
    </tim:Focus>
  </tim:Move>`, videoSourceToken, speed)

	_, err := client.Do(ctx, imagingURL, "", body)
	return err
}

func StopFocus(ctx context.Context, client *SOAPClient, imagingURL, videoSourceToken string) error {
	body := fmt.Sprintf(`<tim:Stop xmlns:tim="http://www.onvif.org/ver20/imaging/wsdl">
    <tim:VideoSourceToken>%s</tim:VideoSourceToken>
  </tim:Stop>`, videoSourceToken)

	_, err := client.Do(ctx, imagingURL, "", body)
	return err
}

func GetImagingStatus(ctx context.Context, client *SOAPClient, imagingURL, videoSourceToken string) (*ImagingStatus, error) {
	body := fmt.Sprintf(`<tim:GetStatus xmlns:tim="http://www.onvif.org/ver20/imaging/wsdl">
    <tim:VideoSourceToken>%s</tim:VideoSourceToken>
  </tim:GetStatus>`, videoSourceToken)

	data, err := client.Do(ctx, imagingURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Status struct {
					FocusState string `xml:"FocusState"`
				} `xml:"ImagingStatus"`
			} `xml:"GetStatusResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return &ImagingStatus{
		FocusState: resp.Body.Response.Status.FocusState,
	}, nil
}

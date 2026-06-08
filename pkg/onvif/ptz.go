package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
)

type PTZPreset struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"Name"`
}

type PTZConfiguration struct {
	Token                     string
	Name                      string
	DefaultPTZSpeed           *PTZSpeed
	DefaultPTZTimeout         string
	PanTiltLimits             *Space2DDescription
	ZoomLimits                *Space1DDescription
}

type PTZSpeed struct {
	PanTilt *Vector2D
	Zoom    *Vector1D
}

type Vector2D struct {
	X     float64 `xml:"x,attr"`
	Y     float64 `xml:"y,attr"`
	Space string  `xml:"space,attr,omitempty"`
}

type Vector1D struct {
	X     float64 `xml:"x,attr"`
	Space string  `xml:"space,attr,omitempty"`
}

type Space2DDescription struct {
	XRange struct {
		Min float64
		Max float64
	}
	YRange struct {
		Min float64
		Max float64
	}
}

type Space1DDescription struct {
	Range struct {
		Min float64
		Max float64
	}
}

type PTZPosition struct {
	PanTilt *Vector2D
	Zoom    *Vector1D
}

type PTZStatus struct {
	Position *PTZPosition `xml:"Position"`
	MoveStatus *struct {
		PanTilt string `xml:"PanTilt"`
		Zoom    string `xml:"Zoom"`
	} `xml:"MoveStatus"`
	UtcTime string `xml:"UtcTime"`
}

func GetPresets(ctx context.Context, client *SOAPClient, ptzURL, profileToken string) ([]PTZPreset, error) {
	body := fmt.Sprintf(`<ptz:GetPresets xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
  </ptz:GetPresets>`, profileToken)

	data, err := client.Do(ctx, ptzURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetPresets failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Presets []struct {
					Token string `xml:"token,attr"`
					Name  string `xml:"Name"`
				} `xml:"Preset"`
			} `xml:"GetPresetsResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetPresets response: %w", err)
	}

	presets := make([]PTZPreset, 0, len(resp.Body.Response.Presets))
	for _, p := range resp.Body.Response.Presets {
		presets = append(presets, PTZPreset{
			Token: p.Token,
			Name:  p.Name,
		})
	}

	return presets, nil
}

func GotoPreset(ctx context.Context, client *SOAPClient, ptzURL, profileToken, presetToken string, speed *PTZSpeed) error {
	speedXML := ""
	if speed != nil {
		speedXML = "<ptz:Speed>"
		if speed.PanTilt != nil {
			speedXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f" space="%s"/>`, speed.PanTilt.X, speed.PanTilt.Y, speed.PanTilt.Space)
		}
		if speed.Zoom != nil {
			speedXML += fmt.Sprintf(`<ptz:Zoom x="%f" space="%s"/>`, speed.Zoom.X, speed.Zoom.Space)
		}
		speedXML += "</ptz:Speed>"
	}

	body := fmt.Sprintf(`<ptz:GotoPreset xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    <ptz:PresetToken>%s</ptz:PresetToken>
    %s
  </ptz:GotoPreset>`, profileToken, presetToken, speedXML)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func SetPreset(ctx context.Context, client *SOAPClient, ptzURL, profileToken, presetName string) (string, error) {
	nameXML := ""
	if presetName != "" {
		nameXML = fmt.Sprintf(`<ptz:PresetName>%s</ptz:PresetName>`, presetName)
	}

	body := fmt.Sprintf(`<ptz:SetPreset xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    %s
  </ptz:SetPreset>`, profileToken, nameXML)

	data, err := client.Do(ctx, ptzURL, "", body)
	if err != nil {
		return "", fmt.Errorf("SetPreset failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return "", err
	}

	token, _ := ExtractXMLString(data, "PresetToken")
	return token, nil
}

func RemovePreset(ctx context.Context, client *SOAPClient, ptzURL, profileToken, presetToken string) error {
	body := fmt.Sprintf(`<ptz:RemovePreset xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    <ptz:PresetToken>%s</ptz:PresetToken>
  </ptz:RemovePreset>`, profileToken, presetToken)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func ContinuousMove(ctx context.Context, client *SOAPClient, ptzURL, profileToken string, panTilt *Vector2D, zoom *Vector1D) error {
	velocityXML := "<ptz:Velocity>"
	if panTilt != nil {
		space := panTilt.Space
		if space == "" {
			space = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocitySpace"
		}
		velocityXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f" space="%s"/>`, panTilt.X, panTilt.Y, space)
	} else {
		velocityXML += `<ptz:PanTilt x="0" y="0" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocitySpace"/>`
	}
	if zoom != nil {
		space := zoom.Space
		if space == "" {
			space = "http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocitySpace"
		}
		velocityXML += fmt.Sprintf(`<ptz:Zoom x="%f" space="%s"/>`, zoom.X, space)
	} else {
		velocityXML += `<ptz:Zoom x="0" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocitySpace"/>`
	}
	velocityXML += "</ptz:Velocity>"

	body := fmt.Sprintf(`<ptz:ContinuousMove xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    %s
  </ptz:ContinuousMove>`, profileToken, velocityXML)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func Stop(ctx context.Context, client *SOAPClient, ptzURL, profileToken string, panTilt, zoom bool) error {
	body := fmt.Sprintf(`<ptz:Stop xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    <ptz:PanTilt>%t</ptz:PanTilt>
    <ptz:Zoom>%t</ptz:Zoom>
  </ptz:Stop>`, profileToken, panTilt, zoom)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func AbsoluteMove(ctx context.Context, client *SOAPClient, ptzURL, profileToken string, position *PTZPosition, speed *PTZSpeed) error {
	positionXML := "<ptz:Position>"
	if position.PanTilt != nil {
		space := position.PanTilt.Space
		if space == "" {
			space = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/PositionSpace"
		}
		positionXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f" space="%s"/>`, position.PanTilt.X, position.PanTilt.Y, space)
	}
	if position.Zoom != nil {
		space := position.Zoom.Space
		if space == "" {
			space = "http://www.onvif.org/ver10/tptz/ZoomSpaces/PositionSpace"
		}
		positionXML += fmt.Sprintf(`<ptz:Zoom x="%f" space="%s"/>`, position.Zoom.X, space)
	}
	positionXML += "</ptz:Position>"

	speedXML := ""
	if speed != nil {
		speedXML = "<ptz:Speed>"
		if speed.PanTilt != nil {
			speedXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocitySpace"/>`, speed.PanTilt.X, speed.PanTilt.Y)
		}
		if speed.Zoom != nil {
			speedXML += fmt.Sprintf(`<ptz:Zoom x="%f" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocitySpace"/>`, speed.Zoom.X)
		}
		speedXML += "</ptz:Speed>"
	}

	body := fmt.Sprintf(`<ptz:AbsoluteMove xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    %s
    %s
  </ptz:AbsoluteMove>`, profileToken, positionXML, speedXML)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func RelativeMove(ctx context.Context, client *SOAPClient, ptzURL, profileToken string, translation *Vector2D, zoom *Vector1D, speed *PTZSpeed) error {
	translationXML := "<ptz:Translation>"
	if translation != nil {
		space := translation.Space
		if space == "" {
			space = "http://www.onvif.org/ver10/tptz/PanTiltSpaces/TranslationSpace"
		}
		translationXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f" space="%s"/>`, translation.X, translation.Y, space)
	}
	if zoom != nil {
		space := zoom.Space
		if space == "" {
			space = "http://www.onvif.org/ver10/tptz/ZoomSpaces/ZoomTranslationSpace"
		}
		translationXML += fmt.Sprintf(`<ptz:Zoom x="%f" space="%s"/>`, zoom.X, space)
	}
	translationXML += "</ptz:Translation>"

	speedXML := ""
	if speed != nil {
		speedXML = "<ptz:Speed>"
		if speed.PanTilt != nil {
			speedXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f"/>`, speed.PanTilt.X, speed.PanTilt.Y)
		}
		speedXML += "</ptz:Speed>"
	}

	body := fmt.Sprintf(`<ptz:RelativeMove xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    %s
    %s
  </ptz:RelativeMove>`, profileToken, translationXML, speedXML)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func GotoHomePosition(ctx context.Context, client *SOAPClient, ptzURL, profileToken string, speed *PTZSpeed) error {
	speedXML := ""
	if speed != nil {
		speedXML = "<ptz:Speed>"
		if speed.PanTilt != nil {
			speedXML += fmt.Sprintf(`<ptz:PanTilt x="%f" y="%f"/>`, speed.PanTilt.X, speed.PanTilt.Y)
		}
		if speed.Zoom != nil {
			speedXML += fmt.Sprintf(`<ptz:Zoom x="%f"/>`, speed.Zoom.X)
		}
		speedXML += "</ptz:Speed>"
	}

	body := fmt.Sprintf(`<ptz:GotoHomePosition xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
    %s
  </ptz:GotoHomePosition>`, profileToken, speedXML)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func SetHomePosition(ctx context.Context, client *SOAPClient, ptzURL, profileToken string) error {
	body := fmt.Sprintf(`<ptz:SetHomePosition xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
  </ptz:SetHomePosition>`, profileToken)

	_, err := client.Do(ctx, ptzURL, "", body)
	return err
}

func GetPTZStatus(ctx context.Context, client *SOAPClient, ptzURL, profileToken string) (*PTZStatus, error) {
	body := fmt.Sprintf(`<ptz:GetStatus xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
  </ptz:GetStatus>`, profileToken)

	data, err := client.Do(ctx, ptzURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetStatus failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				PTZStatus PTZStatus `xml:"PTZStatus"`
			} `xml:"GetStatusResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetStatus response: %w", err)
	}

	return &resp.Body.Response.PTZStatus, nil
}

func GetPTZConfiguration(ctx context.Context, client *SOAPClient, ptzURL, profileToken string) (*PTZConfiguration, error) {
	body := fmt.Sprintf(`<ptz:GetConfiguration xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:ProfileToken>%s</ptz:ProfileToken>
  </ptz:GetConfiguration>`, profileToken)

	data, err := client.Do(ctx, ptzURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetConfiguration failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Config PTZConfiguration `xml:"PTZConfiguration"`
			} `xml:"GetConfigurationResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetConfiguration response: %w", err)
	}

	return &resp.Body.Response.Config, nil
}

func GetNode(ctx context.Context, client *SOAPClient, ptzURL, nodeToken string) (map[string]interface{}, error) {
	body := fmt.Sprintf(`<ptz:GetNode xmlns:ptz="http://www.onvif.org/ver20/ptz/wsdl">
    <ptz:NodeToken>%s</ptz:NodeToken>
  </ptz:GetNode>`, nodeToken)

	data, err := client.Do(ctx, ptzURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Node struct {
					Token       string `xml:"token,attr"`
					Name        string `xml:"Name"`
					SupportedPTZSpaces *struct {
						AbsolutePanTiltPositionSpace []struct {
							URI string `xml:"URI"`
						} `xml:"AbsolutePanTiltPositionSpace"`
						AbsoluteZoomPositionSpace []struct {
							URI string `xml:"URI"`
						} `xml:"AbsoluteZoomPositionSpace"`
						ContinuousPanTiltVelocitySpace []struct {
							URI string `xml:"URI"`
						} `xml:"ContinuousPanTiltVelocitySpace"`
						ContinuousZoomVelocitySpace []struct {
							URI string `xml:"URI"`
						} `xml:"ContinuousZoomVelocitySpace"`
					} `xml:"SupportedPTZSpaces"`
				} `xml:"PTZNode"`
			} `xml:"GetNodeResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"token": resp.Body.Response.Node.Token,
		"name":  resp.Body.Response.Node.Name,
	}

	if spaces := resp.Body.Response.Node.SupportedPTZSpaces; spaces != nil {
		result["has_absolute_pan_tilt"] = len(spaces.AbsolutePanTiltPositionSpace) > 0
		result["has_absolute_zoom"] = len(spaces.AbsoluteZoomPositionSpace) > 0
		result["has_continuous_pan_tilt"] = len(spaces.ContinuousPanTiltVelocitySpace) > 0
		result["has_continuous_zoom"] = len(spaces.ContinuousZoomVelocitySpace) > 0
	}

	return result, nil
}

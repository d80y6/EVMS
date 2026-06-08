package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

type AnalyticsModule struct {
	Token       string
	Name        string
	Type        string
	Parameters  map[string]string
}

type AnalyticsRule struct {
	Token    string
	Name     string
	Type     string
	Enabled  bool
}

type ObjectMetadata struct {
	TrackID     int
	Type        string
	BoundingBox *Rectangle
	Center     *Point
	Speed      *Vector2D
	Confidence float64
	Timestamp  string
}

type Rectangle struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

type Point struct {
	X float64
	Y float64
}

func GetAnalyticsModules(ctx context.Context, client *SOAPClient, analyticsURL string) ([]AnalyticsModule, error) {
	body := `<tana:GetAnalyticsModules xmlns:tana="http://www.onvif.org/ver20/analytics/wsdl"/>`

	data, err := client.Do(ctx, analyticsURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetAnalyticsModules failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Modules []struct {
					Token string `xml:"token,attr"`
					Name  string `xml:"Name"`
					Type  string `xml:"Type"`
				} `xml:"AnalyticsModule"`
			} `xml:"GetAnalyticsModulesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetAnalyticsModules response: %w", err)
	}

	modules := make([]AnalyticsModule, 0, len(resp.Body.Response.Modules))
	for _, m := range resp.Body.Response.Modules {
		modules = append(modules, AnalyticsModule{
			Token: m.Token,
			Name:  m.Name,
			Type:  m.Type,
		})
	}

	return modules, nil
}

func GetSupportedAnalyticsRules(ctx context.Context, client *SOAPClient, analyticsURL string) ([]AnalyticsRule, error) {
	body := `<tana:GetSupportedRules xmlns:tana="http://www.onvif.org/ver20/analytics/wsdl"/>`

	data, err := client.Do(ctx, analyticsURL, "", body)
	if err != nil {
		return nil, fmt.Errorf("GetSupportedRules failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Rules []struct {
					Token string `xml:"token,attr"`
					Name  string `xml:"Name"`
				} `xml:"Rule"`
			} `xml:"GetSupportedRulesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetSupportedRules response: %w", err)
	}

	rules := make([]AnalyticsRule, 0, len(resp.Body.Response.Rules))
	for _, r := range resp.Body.Response.Rules {
		rules = append(rules, AnalyticsRule{
			Token: r.Token,
			Name:  r.Name,
		})
	}

	return rules, nil
}

func ParseAnalyticsMetadata(data []byte) ([]ObjectMetadata, error) {
	var objects []ObjectMetadata
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	var current ObjectMetadata
	inObject := false

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Object":
				inObject = true
				for _, attr := range t.Attr {
					if attr.Name.Local == "ObjectId" {
						fmt.Sscanf(attr.Value, "%d", &current.TrackID)
					}
				}
			case "Appearance":
				for _, attr := range t.Attr {
					if attr.Name.Local == "Class" {
						current.Type = attr.Value
					}
					if attr.Name.Local == "Confidence" {
						fmt.Sscanf(attr.Value, "%f", &current.Confidence)
					}
				}
			case "BoundingBox":
				var box Rectangle
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "left":
						fmt.Sscanf(attr.Value, "%f", &box.Left)
					case "top":
						fmt.Sscanf(attr.Value, "%f", &box.Top)
					case "right":
						fmt.Sscanf(attr.Value, "%f", &box.Right)
					case "bottom":
						fmt.Sscanf(attr.Value, "%f", &box.Bottom)
					}
				}
				current.BoundingBox = &box
			case "Center":
				var pt Point
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "x":
						fmt.Sscanf(attr.Value, "%f", &pt.X)
					case "y":
						fmt.Sscanf(attr.Value, "%f", &pt.Y)
					}
				}
				current.Center = &pt
			case "UtcTime":
				if charToken, err := decoder.Token(); err == nil {
					if chars, ok := charToken.(xml.CharData); ok {
						current.Timestamp = strings.TrimSpace(string(chars))
					}
				}
			}

		case xml.EndElement:
			if t.Name.Local == "Object" && inObject {
				objects = append(objects, current)
				current = ObjectMetadata{}
				inObject = false
			}
		}
	}

	return objects, nil
}

func CreateAnalyticsRule(ctx context.Context, client *SOAPClient, analyticsURL, ruleToken, ruleType string, parameters map[string]string) error {
	paramsXML := ""
	for key, value := range parameters {
		paramsXML += fmt.Sprintf(`<tana:Parameters><tana:SimpleItem Name="%s" Value="%s"/></tana:Parameters>`, key, value)
	}

	body := fmt.Sprintf(`<tana:CreateRule xmlns:tana="http://www.onvif.org/ver20/analytics/wsdl">
    <tana:RuleConfiguration>
      <tana:RuleToken>%s</tana:RuleToken>
      <tana:Type>%s</tana:Type>
      %s
    </tana:RuleConfiguration>
  </tana:CreateRule>`, ruleToken, ruleType, paramsXML)

	_, err := client.Do(ctx, analyticsURL, "", body)
	return err
}

func DeleteAnalyticsRule(ctx context.Context, client *SOAPClient, analyticsURL, ruleToken string) error {
	body := fmt.Sprintf(`<tana:DeleteRule xmlns:tana="http://www.onvif.org/ver20/analytics/wsdl">
    <tana:RuleToken>%s</tana:RuleToken>
  </tana:DeleteRule>`, ruleToken)

	_, err := client.Do(ctx, analyticsURL, "", body)
	return err
}

func GetAnalyticsState(ctx context.Context, client *SOAPClient, analyticsURL string) (map[string]interface{}, error) {
	body := `<tana:GetState xmlns:tana="http://www.onvif.org/ver20/analytics/wsdl"/>`

	data, err := client.Do(ctx, analyticsURL, "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Local == "RuleState" {
				if charToken, err := decoder.Token(); err == nil {
					if chars, ok := charToken.(xml.CharData); ok {
						result["rule_state"] = strings.TrimSpace(string(chars))
					}
				}
			}
		}
	}

	return result, nil
}

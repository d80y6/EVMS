package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SOAPClient struct {
	httpClient *http.Client
	creds      *Credentials
	userAgent  string
}

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Header  *SOAPHeader
	Body    SOAPBody
}

type SOAPHeader struct {
	XMLName     xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Header"`
	Security    string   `xml:",innerxml"`
}

type SOAPBody struct {
	XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Body"`
	Content string   `xml:",innerxml"`
}

type SOAPFault struct {
	XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Fault"`
	Code    string   `xml:"Code>Value"`
	Reason  string   `xml:"Reason>Text"`
	Detail  string   `xml:"Detail>text"`
}

func (f *SOAPFault) Error() string {
	return fmt.Sprintf("SOAP fault: %s - %s", f.Code, f.Reason)
}

func NewSOAPClient(timeout time.Duration, creds *Credentials) *SOAPClient {
	return &SOAPClient{
		httpClient: &http.Client{Timeout: timeout},
		creds:      creds,
		userAgent:  "EVMS/1.0",
	}
}

func (c *SOAPClient) Do(ctx context.Context, url, action string, bodyXML string) ([]byte, error) {
	var soapBody string
	var wst *WSUsernameToken
	if c.creds != nil && c.creds.Username != "" {
		wst = NewWSUsernameToken(c.creds.Username, c.creds.Password)
	}

	if wst != nil {
		soapBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Header>
    %s
  </soap:Header>
  <soap:Body>
    %s
  </soap:Body>
</soap:Envelope>`, wst.SOAPHeader(), bodyXML)
	} else {
		soapBody = fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
  <soap:Body>
    %s
  </soap:Body>
</soap:Envelope>`, bodyXML)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("User-Agent", c.userAgent)
	if action != "" {
		req.Header.Set("SOAPAction", action)
	}

	ApplyAuth(req, c.creds)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		var fault SOAPFault
		if err := xml.Unmarshal(data, &fault); err == nil && fault.Code != "" {
			return data, &fault
		}
		return data, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func ParseSOAPResponse(data []byte, target interface{}) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("XML token error: %w", err)
		}
		if start, ok := token.(xml.StartElement); ok {
			if start.Name.Local == "Body" {
				return decoder.Decode(target)
			}
		}
	}
	return fmt.Errorf("no SOAP Body found in response")
}

func ExtractFault(data []byte) *SOAPFault {
	var fault SOAPFault
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := token.(xml.StartElement); ok {
			switch start.Name.Local {
			case "Value":
				if charToken, err := decoder.Token(); err == nil {
					if chars, ok := charToken.(xml.CharData); ok {
						fault.Code = strings.TrimSpace(string(chars))
					}
				}
			case "Text":
				if charToken, err := decoder.Token(); err == nil {
					if chars, ok := charToken.(xml.CharData); ok {
						fault.Reason = strings.TrimSpace(string(chars))
					}
				}
			case "Detail":
				if charToken, err := decoder.Token(); err == nil {
					if chars, ok := charToken.(xml.CharData); ok {
						fault.Detail = strings.TrimSpace(string(chars))
					}
				}
			}
		}
	}

	if fault.Code != "" || fault.Reason != "" {
		return &fault
	}
	return nil
}

func CheckSOAPFault(data []byte) error {
	if fault := ExtractFault(data); fault != nil {
		return fault
	}
	return nil
}

func toHTTPURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	switch u.Scheme {
	case "rtsp":
		u.Scheme = "http"
		if u.Port() == "554" {
			u.Host = u.Hostname() + ":80"
		}
	case "rtsps":
		u.Scheme = "https"
		if u.Port() == "322" {
			u.Host = u.Hostname() + ":443"
		}
	}
	return u.String()
}

func BuildMediaURL(baseURL string) string {
	return strings.TrimRight(toHTTPURL(baseURL), "/") + "/onvif/media_service"
}

func BuildPTZURL(baseURL string) string {
	return strings.TrimRight(toHTTPURL(baseURL), "/") + "/onvif/ptz_service"
}

func BuildDeviceURL(baseURL string) string {
	return strings.TrimRight(toHTTPURL(baseURL), "/") + "/onvif/device_service"
}

func BuildEventURL(baseURL string) string {
	return strings.TrimRight(toHTTPURL(baseURL), "/") + "/onvif/event_service"
}

func BuildImagingURL(baseURL string) string {
	return strings.TrimRight(toHTTPURL(baseURL), "/") + "/onvif/imaging_service"
}

func BuildRecordingURL(baseURL string) string {
	return strings.TrimRight(toHTTPURL(baseURL), "/") + "/onvif/recording_service"
}

func BuildReplayURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/onvif/replay_service"
}

func BuildAnalyticsURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/onvif/analytics_service"
}

func BuildDeviceIOURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/onvif/deviceio_service"
}

func ExtractXMLString(data []byte, tag string) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == tag {
			charToken, err := decoder.Token()
			if err != nil {
				return "", err
			}
			if chars, ok := charToken.(xml.CharData); ok {
				return strings.TrimSpace(string(chars)), nil
			}
		}
	}
	return "", fmt.Errorf("tag %s not found", tag)
}

type SOAPBuilder struct {
	namespaces map[string]string
	header     string
}

func NewSOAPBuilder() *SOAPBuilder {
	return &SOAPBuilder{
		namespaces: map[string]string{
			"soap": "http://www.w3.org/2003/05/soap-envelope",
		},
	}
}

func (b *SOAPBuilder) WithNamespace(prefix, uri string) *SOAPBuilder {
	b.namespaces[prefix] = uri
	return b
}

func (b *SOAPBuilder) WithHeader(xml string) *SOAPBuilder {
	b.header = xml
	return b
}

func (b *SOAPBuilder) Build(body string) string {
	var nsDecls []string
	for prefix, uri := range b.namespaces {
		nsDecls = append(nsDecls, fmt.Sprintf(`xmlns:%s="%s"`, prefix, uri))
	}

	if b.header != "" {
		return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope %s>
  <soap:Header>
    %s
  </soap:Header>
  <soap:Body>
    %s
  </soap:Body>
</soap:Envelope>`, strings.Join(nsDecls, " "), b.header, body)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope %s>
  <soap:Body>
    %s
  </soap:Body>
</soap:Envelope>`, strings.Join(nsDecls, " "), body)
}

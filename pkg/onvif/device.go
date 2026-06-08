package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

type DeviceInfo struct {
	Manufacturer    string
	Model           string
	FirmwareVersion string
	SerialNumber    string
	HardwareID      string
}

type Capabilities struct {
	Analytics bool
	Device    bool
	Events    bool
	Imaging   bool
	Media     bool
	PTZ       bool
	Recording bool
	Replay    bool
	TLS       bool
	Extension map[string]string
}

type Service struct {
	Namespace string
	XAddr     string
	Version   string
}

func GetDeviceInformation(ctx context.Context, client *SOAPClient, deviceURL string) (*DeviceInfo, error) {
	body := `<tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, fmt.Errorf("GetDeviceInformation failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Manufacturer    string `xml:"Manufacturer"`
				Model           string `xml:"Model"`
				FirmwareVersion string `xml:"FirmwareVersion"`
				SerialNumber    string `xml:"SerialNumber"`
				HardwareID      string `xml:"HardwareId"`
			} `xml:"GetDeviceInformationResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetDeviceInformation response: %w", err)
	}

	return &DeviceInfo{
		Manufacturer:    resp.Body.Response.Manufacturer,
		Model:           resp.Body.Response.Model,
		FirmwareVersion: resp.Body.Response.FirmwareVersion,
		SerialNumber:    resp.Body.Response.SerialNumber,
		HardwareID:      resp.Body.Response.HardwareID,
	}, nil
}

func GetCapabilities(ctx context.Context, client *SOAPClient, deviceURL string) (*Capabilities, error) {
	body := `<tds:GetCapabilities xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
    <tds:Category>All</tds:Category>
  </tds:GetCapabilities>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, fmt.Errorf("GetCapabilities failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Capabilities struct {
					Analytics *struct {
						XAddr string `xml:"XAddr,attr"`
					} `xml:"Analytics"`
					Device *struct {
						XAddr string `xml:"XAddr,attr"`
					} `xml:"Device"`
					Events *struct {
						XAddr string `xml:"XAddr,attr"`
					} `xml:"Events"`
					Imaging *struct {
						XAddr string `xml:"XAddr,attr"`
					} `xml:"Imaging"`
					Media *struct {
						XAddr       string `xml:"XAddr,attr"`
						Streaming   bool   `xml:"Streaming,attr"`
						SnapshotUri bool   `xml:"SnapshotUri,attr"`
					} `xml:"Media"`
					PTZ *struct {
						XAddr string `xml:"XAddr,attr"`
					} `xml:"PTZ"`
					Recording *struct {
						XAddr  string `xml:"XAddr,attr"`
					} `xml:"Recording"`
					Replay *struct {
						XAddr string `xml:"XAddr,attr"`
					} `xml:"Replay"`
				} `xml:"Capabilities"`
			} `xml:"GetCapabilitiesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetCapabilities response: %w", err)
	}

	caps := &Capabilities{
		Extension: make(map[string]string),
	}

	if resp.Body.Response.Capabilities.Analytics != nil {
		caps.Analytics = true
	}
	if resp.Body.Response.Capabilities.Device != nil {
		caps.Device = true
	}
	if resp.Body.Response.Capabilities.Events != nil {
		caps.Events = true
	}
	if resp.Body.Response.Capabilities.Imaging != nil {
		caps.Imaging = true
	}
	if resp.Body.Response.Capabilities.Media != nil {
		caps.Media = true
	}
	if resp.Body.Response.Capabilities.PTZ != nil {
		caps.PTZ = true
	}
	if resp.Body.Response.Capabilities.Recording != nil {
		caps.Recording = true
	}
	if resp.Body.Response.Capabilities.Replay != nil {
		caps.Replay = true
	}

	return caps, nil
}

func GetServices(ctx context.Context, client *SOAPClient, deviceURL string) ([]Service, error) {
	body := `<tds:GetServices xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
    <tds:IncludeCapability>true</tds:IncludeCapability>
  </tds:GetServices>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, fmt.Errorf("GetServices failed: %w", err)
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Services []struct {
					Namespace string `xml:"Namespace"`
					XAddr     string `xml:"XAddr"`
					Version   struct {
						Major int `xml:"Major"`
						Minor int `xml:"Minor"`
					} `xml:"Version"`
				} `xml:"Service"`
			} `xml:"GetServicesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse GetServices response: %w", err)
	}

	services := make([]Service, 0, len(resp.Body.Response.Services))
	for _, s := range resp.Body.Response.Services {
		services = append(services, Service{
			Namespace: s.Namespace,
			XAddr:     s.XAddr,
			Version:   fmt.Sprintf("%d.%d", s.Version.Major, s.Version.Minor),
		})
	}

	return services, nil
}

func GetSystemDateAndTime(ctx context.Context, client *SOAPClient, deviceURL string) (map[string]interface{}, error) {
	body := `<tds:GetSystemDateAndTime xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	if utc, err := ExtractXMLString(data, "UTCDateTime"); err == nil {
		result["utc_date_time"] = utc
	}
	if tz, err := ExtractXMLString(data, "TZ"); err == nil {
		result["timezone"] = tz
	}
	if dst, err := ExtractXMLString(data, "DaylightSavings"); err == nil {
		result["daylight_savings"] = dst
	}

	return result, nil
}

func GetNetworkInterfaces(ctx context.Context, client *SOAPClient, deviceURL string) ([]map[string]interface{}, error) {
	body := `<tds:GetNetworkInterfaces xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	var resp struct {
		Body struct {
			Response struct {
				Interfaces []struct {
					Name string `xml:"Name"`
					HwAddr string `xml:"HwAddress"`
					MTU    int    `xml:"MTU"`
					IPv4   *struct {
						Enabled bool   `xml:"Enabled"`
						Address  string `xml:"Address"`
						Prefix   int    `xml:"PrefixLength"`
					} `xml:"IPv4"`
				} `xml:"NetworkInterfaces"`
			} `xml:"GetNetworkInterfacesResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	interfaces := make([]map[string]interface{}, 0, len(resp.Body.Response.Interfaces))
	for _, iface := range resp.Body.Response.Interfaces {
		entry := map[string]interface{}{
			"name":   iface.Name,
			"mac":    iface.HwAddr,
			"mtu":    iface.MTU,
		}
		if iface.IPv4 != nil {
			entry["ipv4_enabled"] = iface.IPv4.Enabled
			entry["ipv4_address"] = iface.IPv4.Address
			entry["ipv4_prefix"] = iface.IPv4.Prefix
		}
		interfaces = append(interfaces, entry)
	}

	return interfaces, nil
}

func GetDNS(ctx context.Context, client *SOAPClient, deviceURL string) (map[string]interface{}, error) {
	body := `<tds:GetDNS xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	if fromDHCP, err := ExtractXMLString(data, "FromDHCP"); err == nil {
		result["from_dhcp"] = fromDHCP
	}
	var dnsServers []string
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "DNSManual" {
			if charToken, err := decoder.Token(); err == nil {
				if chars, ok := charToken.(xml.CharData); ok {
					dnsServers = append(dnsServers, strings.TrimSpace(string(chars)))
				}
			}
		}
	}
	result["dns_servers"] = dnsServers

	return result, nil
}

func GetNTP(ctx context.Context, client *SOAPClient, deviceURL string) (map[string]interface{}, error) {
	body := `<tds:GetNTP xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return nil, err
	}

	if err := CheckSOAPFault(data); err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	if fromDHCP, err := ExtractXMLString(data, "FromDHCP"); err == nil {
		result["from_dhcp"] = fromDHCP
	}

	return result, nil
}

func GetHostname(ctx context.Context, client *SOAPClient, deviceURL string) (string, error) {
	body := `<tds:GetHostname xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	data, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	if err != nil {
		return "", err
	}

	if err := CheckSOAPFault(data); err != nil {
		return "", err
	}

	return ExtractXMLString(data, "Name")
}

func Reboot(ctx context.Context, client *SOAPClient, deviceURL string) error {
	body := `<tds:SystemReboot xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>`

	_, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	return err
}

func SetHostname(ctx context.Context, client *SOAPClient, deviceURL, hostname string) error {
	body := fmt.Sprintf(`<tds:SetHostname xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
    <tds:Name>%s</tds:Name>
  </tds:SetHostname>`, hostname)

	_, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	return err
}

func SetDNS(ctx context.Context, client *SOAPClient, deviceURL string, fromDHCP bool, dnsServers []string) error {
	body := fmt.Sprintf(`<tds:SetDNS xmlns:tds="http://www.onvif.org/ver10/device/wsdl">
    <tds:FromDHCP>%t</tds:FromDHCP>`, fromDHCP)
	for _, s := range dnsServers {
		body += fmt.Sprintf(`<tds:DNSManual>%s</tds:DNSManual>`, s)
	}
	body += `</tds:SetDNS>`

	_, err := client.Do(ctx, BuildDeviceURL(deviceURL), "", body)
	return err
}

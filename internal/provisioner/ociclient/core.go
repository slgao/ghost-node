package ociclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// apiVersion is the Core Services API version segment.
const apiVersion = "/20160918"

// Public IP lifecycle states.
const (
	PublicIPAssigned   = "ASSIGNED"
	PublicIPAvailable  = "AVAILABLE"
	PublicIPAssigning  = "ASSIGNING"
	PublicIPTerminated = "TERMINATED"
)

// Public IP lifetimes.
const (
	LifetimeEphemeral = "EPHEMERAL"
	LifetimeReserved  = "RESERVED"
)

type Instance struct {
	ID                 string `json:"id"`
	CompartmentID      string `json:"compartmentId"`
	DisplayName        string `json:"displayName"`
	LifecycleState     string `json:"lifecycleState"`
	Region             string `json:"region"`
	AvailabilityDomain string `json:"availabilityDomain"`
	Shape              string `json:"shape"`
}

type VnicAttachment struct {
	ID             string `json:"id"`
	VnicID         string `json:"vnicId"`
	InstanceID     string `json:"instanceId"`
	LifecycleState string `json:"lifecycleState"`
}

type Vnic struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	PrivateIP   string `json:"privateIp"`
	PublicIP    string `json:"publicIp"`
	IsPrimary   bool   `json:"isPrimary"`
	SubnetID    string `json:"subnetId"`
}

type PrivateIP struct {
	ID            string `json:"id"`
	CompartmentID string `json:"compartmentId"`
	IPAddress     string `json:"ipAddress"`
	IsPrimary     bool   `json:"isPrimary"`
	VnicID        string `json:"vnicId"`
}

type PublicIP struct {
	ID             string `json:"id"`
	CompartmentID  string `json:"compartmentId"`
	DisplayName    string `json:"displayName"`
	IPAddress      string `json:"ipAddress"`
	Lifetime       string `json:"lifetime"`
	LifecycleState string `json:"lifecycleState"`
	PrivateIPID    string `json:"privateIpId"`
}

// GetInstance fetches an instance by OCID.
func (c *Client) GetInstance(ctx context.Context, instanceOCID string) (*Instance, error) {
	var out Instance
	path := apiVersion + "/instances/" + url.PathEscape(instanceOCID)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVnicAttachments lists the VNIC attachments of an instance.
func (c *Client) ListVnicAttachments(ctx context.Context, compartmentOCID, instanceOCID string) ([]VnicAttachment, error) {
	q := url.Values{}
	q.Set("compartmentId", compartmentOCID)
	q.Set("instanceId", instanceOCID)

	var out []VnicAttachment
	if err := c.do(ctx, http.MethodGet, apiVersion+"/vnicAttachments", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetVnic fetches a VNIC by OCID.
func (c *Client) GetVnic(ctx context.Context, vnicOCID string) (*Vnic, error) {
	var out Vnic
	path := apiVersion + "/vnics/" + url.PathEscape(vnicOCID)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPrivateIPs lists the private IPs assigned to a VNIC.
func (c *Client) ListPrivateIPs(ctx context.Context, vnicOCID string) ([]PrivateIP, error) {
	q := url.Values{}
	q.Set("vnicId", vnicOCID)

	var out []PrivateIP
	if err := c.do(ctx, http.MethodGet, apiVersion+"/privateIps", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPublicIPByPrivateIPID returns the public IP attached to a private IP.
// Returns a 404 APIError (see IsNotFound) when nothing is attached.
func (c *Client) GetPublicIPByPrivateIPID(ctx context.Context, privateIPOCID string) (*PublicIP, error) {
	in := map[string]string{"privateIpId": privateIPOCID}

	var out PublicIP
	path := apiVersion + "/publicIps/actions/getByPrivateIpId"
	if err := c.do(ctx, http.MethodPost, path, nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPublicIP fetches a public IP by OCID.
func (c *Client) GetPublicIP(ctx context.Context, publicIPOCID string) (*PublicIP, error) {
	var out PublicIP
	path := apiVersion + "/publicIps/" + url.PathEscape(publicIPOCID)
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePublicIP releases a public IP. For an ephemeral IP this unassigns it
// from its private IP and returns the address to Oracle's regional pool —
// which is exactly what makes a burned address recoverable-from.
func (c *Client) DeletePublicIP(ctx context.Context, publicIPOCID string) error {
	path := apiVersion + "/publicIps/" + url.PathEscape(publicIPOCID)
	return c.do(ctx, http.MethodDelete, path, nil, nil, nil)
}

// CreateEphemeralPublicIP allocates a fresh ephemeral public IP on a private IP.
// compartmentOCID must be the private IP's compartment — Oracle rejects any other.
func (c *Client) CreateEphemeralPublicIP(ctx context.Context, compartmentOCID, privateIPOCID, displayName string) (*PublicIP, error) {
	in := map[string]string{
		"compartmentId": compartmentOCID,
		"lifetime":      LifetimeEphemeral,
		"privateIpId":   privateIPOCID,
	}
	if displayName != "" {
		in["displayName"] = displayName
	}

	var out PublicIP
	if err := c.do(ctx, http.MethodPost, apiVersion+"/publicIps", nil, in, &out); err != nil {
		return nil, fmt.Errorf("allocating ephemeral public IP: %w", err)
	}
	return &out, nil
}

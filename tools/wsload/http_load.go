package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func metricGauge(text, name string) float64 {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == name {
			v, _ := strconv.ParseFloat(fields[1], 64)
			return v
		}
	}
	return -1
}

// Never include response bodies or presigned URLs in errors or raw data.
func requestJSON(ctx context.Context, client *http.Client, method, addr, token string, body, result any) error {
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, addr, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("invalid HTTP endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return fmt.Errorf("HTTP body failed or too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if result != nil {
		if err = json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("invalid HTTP JSON")
		}
	}
	return nil
}

func lookupChat(ctx context.Context, c config, u user) error {
	var result struct {
		ID string `json:"chatId"`
	}
	body := map[string]any{"target_user_id": u.peerID, "encrypted_chat_key": map[string]string{u.id: "load-key", u.peerID: "load-key"},
		"wrapped_for_public_key": map[string]string{u.id: u.publicKey, u.peerID: u.peerPublicKey}}
	if err := requestJSON(ctx, httpClient, "POST", c.chatURL+"/v1/chats", u.token, body, &result); err != nil {
		return err
	}
	if result.ID != u.chatID {
		return fmt.Errorf("existing private chat changed")
	}
	return nil
}

func mediaClient(minio string) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tr := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "minio:9000" {
			addr = minio
		}
		return dialer.DialContext(ctx, network, addr)
	}}
	return &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func object(ctx context.Context, client *http.Client, method, addr string, data []byte) ([]byte, error) {
	u, err := url.Parse(addr)
	if err != nil || u.Scheme != "http" || u.Host != "minio:9000" || !strings.HasPrefix(u.Path, "/messenger-media/") {
		return nil, fmt.Errorf("unexpected object URL")
	}
	req, err := http.NewRequestWithContext(ctx, method, addr, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid object request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("object request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("object HTTP %d", resp.StatusCode)
	}
	got, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(got) > 4<<20 {
		return nil, fmt.Errorf("invalid object body")
	}
	return got, nil
}

func transferMedia(ctx context.Context, client *http.Client, c config, u user, id string) (int64, error) {
	data := bytes.Repeat([]byte(id+":"), c.mediaBytes/len(id+":")+1)[:c.mediaBytes]
	var ticket struct {
		ID  string `json:"uploadId"`
		URL string `json:"uploadUrl"`
	}
	if err := requestJSON(ctx, client, "POST", c.mediaURL+"/v1/media/uploads", u.token, map[string]any{"chat_id": u.chatID, "content_type": "application/octet-stream", "size_bytes": len(data)}, &ticket); err != nil {
		return 0, err
	}
	if ticket.ID == "" {
		return 0, fmt.Errorf("upload without ID")
	}
	if _, err := object(ctx, client, "PUT", ticket.URL, data); err != nil {
		return 0, err
	}
	var confirmed struct {
		ID string `json:"mediaId"`
	}
	if err := requestJSON(ctx, client, "POST", c.mediaURL+"/v1/media/uploads/"+ticket.ID+"/confirm", u.token, map[string]any{}, &confirmed); err != nil {
		return 0, err
	}
	if confirmed.ID == "" {
		return 0, fmt.Errorf("confirmation without ID")
	}
	var download struct {
		URL string `json:"downloadUrl"`
	}
	if err := requestJSON(ctx, client, "GET", c.mediaURL+"/v1/media/"+confirmed.ID+"/download-url", u.peerToken, nil, &download); err != nil {
		return 0, err
	}
	got, err := object(ctx, client, "GET", download.URL, nil)
	if err != nil {
		return 0, err
	}
	if !bytes.Equal(got, data) {
		return 0, fmt.Errorf("download differs")
	}
	return int64(len(got)), nil
}

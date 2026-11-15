package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type fixtureUser struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	ChatID    string `json:"chat_id"`
	PeerID    string `json:"peer_id"`
	PublicKey string `json:"public_key"`
	Gateway   int    `json:"gateway"`
}

type fixture struct {
	Schema int           `json:"schema"`
	Users  []fixtureUser `json:"users"`
}

func readFixture(path string, gateways int) ([][]user, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open fixture: %w", err)
	}
	defer func() { _ = f.Close() }()
	var data fixture
	d := json.NewDecoder(io.LimitReader(f, 8<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(&data); err != nil {
		return nil, fmt.Errorf("invalid fixture JSON")
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing fixture JSON")
	}
	if data.Schema != 1 || gateways != 2 || len(data.Users) == 0 {
		return nil, fmt.Errorf("fixture requires schema 1, two gateways and reciprocal user pairs")
	}
	byID := map[string]fixtureUser{}
	for _, u := range data.Users {
		if u.ID == "" || u.Token == "" || u.ChatID == "" || u.PeerID == "" || u.ID == u.PeerID || u.Gateway < 0 || u.Gateway >= gateways {
			return nil, fmt.Errorf("incomplete fixture user")
		}
		if _, ok := byID[u.ID]; ok {
			return nil, fmt.Errorf("duplicate fixture user")
		}
		byID[u.ID] = u
	}
	result := make([][]user, gateways)
	for _, u := range data.Users {
		peer, ok := byID[u.PeerID]
		if !ok || peer.PeerID != u.ID || peer.ChatID != u.ChatID || peer.Gateway == u.Gateway {
			return nil, fmt.Errorf("fixture pair must share a chat across gateways")
		}
		result[u.Gateway] = append(result[u.Gateway], user{id: u.ID, token: u.Token, chatID: u.ChatID, peerID: u.PeerID, peerToken: peer.Token, publicKey: u.PublicKey, peerPublicKey: peer.PublicKey})
	}
	if len(result[0]) != len(result[1]) {
		return nil, fmt.Errorf("unequal fixture gateway counts")
	}
	return result, nil
}

// Registry readiness is outside the measurement window and uses no JWT.
func fixtureReady(ctx context.Context, c config) error {
	for _, gw := range c.gateways {
		u, err := url.Parse(gw.url)
		if err != nil {
			return err
		}
		if u.Scheme == "ws" {
			u.Scheme = "http"
		} else {
			u.Scheme = "https"
		}
		u.Path, u.RawQuery = "/metrics", ""
		for {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return err
			}
			resp, err := httpClient.Do(req)
			if err == nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
				_ = resp.Body.Close()
				if readErr == nil && resp.StatusCode == 200 && metricGauge(string(body), "ws_connections_active") == float64(c.users) {
					break
				}
			}
			timer := time.NewTimer(20 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil
}

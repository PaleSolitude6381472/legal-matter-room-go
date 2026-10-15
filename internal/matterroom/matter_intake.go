package matterroom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

type MatterIntake struct {
	MatterID       string    `json:"matter_id"`
	ClientID       string    `json:"client_id"`
	ClientName     string    `json:"client_name"`
	Deadline       time.Time `json:"deadline"`
	DocumentSigned bool      `json:"document_signed"`
}

type Assignment struct {
	MatterID      string `json:"matter_id"`
	Queue         string `json:"queue"`
	Channel       string `json:"channel"`
	Room          string `json:"room"`
	RealtimeToken string `json:"realtime_token"`
	RTCToken      string `json:"rtc_token"`
}

type tokenData struct {
	Token string `json:"token"`
}

type Workflow struct {
	client *Client
	now    func() time.Time
}

func NewWorkflow(client *Client) *Workflow {
	return &Workflow{client: client, now: time.Now}
}

func (w *Workflow) Match(ctx context.Context, intake MatterIntake) (Assignment, error) {
	if intake.MatterID == "" || intake.ClientID == "" || intake.ClientName == "" || intake.Deadline.IsZero() {
		return Assignment{}, fmt.Errorf("matter_id, client_id, client_name, and deadline are required")
	}

	queue := decideQueue(w.now(), intake)
	channel := "matter-" + intake.MatterID
	room := "consult-" + intake.MatterID
	key := stableKey(intake.MatterID)

	if err := w.client.post(ctx, "/v1/realtime/channel/create", struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
		Vendor  string `json:"vendor"`
	}{channel, "private", "auto"}, key+":channel", nil); err != nil {
		return Assignment{}, err
	}
	if err := w.client.post(ctx, "/v1/rtc/room/create", struct {
		Name            string `json:"name"`
		MaxParticipants int    `json:"max_participants"`
		EmptyTimeoutS   int    `json:"empty_timeout_s"`
		Region          string `json:"region"`
	}{room, 4, 900, "auto"}, key+":room", nil); err != nil {
		return Assignment{}, err
	}
	state := struct {
		MatterID       string `json:"matter_id"`
		Queue          string `json:"queue"`
		DocumentSigned bool   `json:"document_signed"`
		Deadline       string `json:"deadline"`
	}{intake.MatterID, queue, intake.DocumentSigned, intake.Deadline.UTC().Format(time.RFC3339)}
	if err := w.client.post(ctx, "/v1/realtime/publish", struct {
		Channel   string `json:"channel"`
		Event     string `json:"event"`
		Data      any    `json:"data"`
		AccountID string `json:"account_id"`
	}{channel, "matter.matched", state, intake.ClientID}, key+":publish", nil); err != nil {
		return Assignment{}, err
	}

	var realtimeToken tokenData
	if err := w.client.post(ctx, "/v1/realtime/token/issue", struct {
		ClientID     string   `json:"client_id"`
		Channels     []string `json:"channels"`
		Capabilities []string `json:"capabilities"`
		TTLSeconds   int      `json:"ttl_seconds"`
	}{intake.ClientID, []string{channel}, []string{"subscribe"}, 3600}, key+":realtime-token", &realtimeToken); err != nil {
		return Assignment{}, err
	}
	var rtcToken tokenData
	if err := w.client.post(ctx, "/v1/rtc/token/issue", struct {
		Room         string `json:"room"`
		Identity     string `json:"identity"`
		DisplayName  string `json:"display_name"`
		TTLS         int    `json:"ttl_s"`
		CanPublish   bool   `json:"can_publish"`
		CanSubscribe bool   `json:"can_subscribe"`
	}{room, intake.ClientID, intake.ClientName, 3600, true, true}, key+":rtc-token", &rtcToken); err != nil {
		return Assignment{}, err
	}

	return Assignment{
		MatterID: intake.MatterID, Queue: queue, Channel: channel, Room: room,
		RealtimeToken: realtimeToken.Token, RTCToken: rtcToken.Token,
	}, nil
}

func decideQueue(now time.Time, intake MatterIntake) string {
	if !intake.DocumentSigned {
		return "awaiting-signature"
	}
	if !intake.Deadline.After(now.Add(48 * time.Hour)) {
		return "deadline-priority"
	}
	return "standard-review"
}

func stableKey(matterID string) string {
	sum := sha256.Sum256([]byte("matter-match:" + matterID))
	return hex.EncodeToString(sum[:16])
}

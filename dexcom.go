package dexgo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

const dexcomBaseUrl = "https://share2.dexcom.com/ShareWebServices/Services"

const dexcomLoginEndpoint = "General/LoginPublisherAccountById"
const dexcomAuthEndpoint = "General/AuthenticatePublisherAccount"
const dexcomGetLatestEndpoint = "Publisher/ReadPublisherLatestGlucoseValues"
const dexcomApplicationId = "d89443d2-327c-4a6f-89e5-496bbb0317db"

var timestampRegex = regexp.MustCompile(`Date\((\d*)\)`)

type GlucoseReading struct {
	Time  time.Time
	Value int
	Trend string
}

type VerifyPayload struct {
	AccountName   string `json:"accountName"`
	Password      string `json:"password"`
	ApplicationId string `json:"applicationId"`
}

type AuthPayload struct {
	AccountId     string `json:"accountId"`
	Password      string `json:"password"`
	ApplicationId string `json:"applicationId"`
}

type GetReadingsPayload struct {
	SessionId string `json:"sessionId"`
	Minutes   int    `json:"minutes"`
	MaxCount  int    `json:"maxCount"`
}

type RawReading struct {
	Time  string `json:"WT"`
	Trend string `json:"Trend"`
	Value int    `json:"Value"`
}

type Dexcom struct {
	username  string
	password  string
	accountId *string
	sessionId *string
}

func New(username string, password string) Dexcom {
	return Dexcom{username: username, password: password}
}

func request(endPoint string, payload any, result interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	authUrl, _ := url.JoinPath(dexcomBaseUrl, endPoint)
	resp, err := http.Post(authUrl, "application/json", bytes.NewBuffer(payloadJSON))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, result)
	return err
}

func (d *Dexcom) fetchAccountId() error {
	payload := VerifyPayload{AccountName: d.username, Password: d.password, ApplicationId: dexcomApplicationId}
	var accountId string
	err := request(dexcomAuthEndpoint, payload, &accountId)
	if err != nil {
		return err
	}
	d.accountId = &accountId
	return nil
}

func (d *Dexcom) auth() error {
	payload := AuthPayload{AccountId: *d.accountId, Password: d.password, ApplicationId: dexcomApplicationId}
	var sessionId string
	err := request(dexcomLoginEndpoint, payload, &sessionId)
	if err != nil {
		return err
	}
	d.sessionId = &sessionId
	return nil
}

func convertTimestamp(wt string) (time.Time, error) {
	matches := timestampRegex.FindStringSubmatch(wt)
	if len(matches) != 2 {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %s", wt)
	}
	timeMillis, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timestamp: %s", matches[1])
	}
	return time.UnixMilli(int64(timeMillis)), nil
}

func (d *Dexcom) GetReadings(minutes int, numReadings int) ([]GlucoseReading, error) {
	if d.sessionId == nil {
		if d.accountId == nil {
			d.fetchAccountId()
		}
		d.auth()
	}
	payload := GetReadingsPayload{
		SessionId: *d.sessionId,
		Minutes:   minutes,
		MaxCount:  numReadings,
	}
	var rawValues []RawReading
	err := request(dexcomGetLatestEndpoint, payload, &rawValues)
	if err != nil {
		return nil, err
	}
	var readings []GlucoseReading
	for _, rv := range rawValues {
		timestamp, err := convertTimestamp(rv.Time)
		if err != nil {
			return nil, err
		}
		r := GlucoseReading{
			Trend: rv.Trend,
			Value: rv.Value,
			Time:  timestamp,
		}
		readings = append(readings, r)
	}
	return readings, nil
}

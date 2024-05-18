package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/astromechza/surehub-prob-exporter/client"
	"github.com/astromechza/surehub-prob-exporter/ref"
)

const (
	promNamespace = "surehub"
)

var jwtParser = jwt.NewParser()

var eventType = map[int]string{
	0:  "UNKNOWN",
	21: "FOOD_FILLED",
	22: "EAT",
	24: "FEEDER_TARE",
}

type Poller struct {
	Client   client.ClientWithResponsesInterface
	Interval time.Duration

	HubEmail    string
	HubPassword string

	token          string
	pollError      error
	lastTimelineId int64
}

func (p *Poller) Start(ctx context.Context) error {
	if p.Client == nil {
		return errors.New("client not configured")
	} else if p.Interval <= time.Second {
		return errors.New("interval not configured")
	} else if p.HubEmail == "" || p.HubPassword == "" {
		return errors.New("hub credentials not configured")
	}

	if err := p.login(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if err := p.poll(ctx); err != nil {
		return fmt.Errorf("first poll failed: %w", err)
	}
	slog.Info("first poll succeeded, now looping in background", "interval", p.Interval)

	go func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("fatal error", "error", p)
				os.Exit(1)
			}
		}()

		t := time.NewTicker(p.Interval)
		for {
			select {
			case <-t.C:
				if err := p.poll(ctx); err != nil {
					slog.Error("poll failed", "error", err)
					p.pollError = err
				} else {
					slog.Info("poll succeeded")
					p.pollError = nil
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (p *Poller) UnreadyError() error {
	return p.pollError
}

func (p *Poller) login(ctx context.Context) error {
	res, err := p.Client.PostApiAuthLoginWithResponse(ctx, &client.PostApiAuthLoginParams{}, client.PostApiAuthLoginJSONRequestBody{
		ClientUid:    uuid.NewString(),
		EmailAddress: openapi_types.Email(p.HubEmail),
		Password:     p.HubPassword,
	})
	if err != nil {
		return fmt.Errorf("failed to make login request: %w", err)
	} else if res.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status code from login: %d %s", res.StatusCode(), string(res.Body))
	} else if res.JSON200.Data.Token == "" {
		slog.Debug("unexpected response body", "body", string(res.Body))
		return errors.New("unexpected login response - empty token")
	} else if j, _, err := jwtParser.ParseUnverified(res.JSON200.Data.Token, jwt.MapClaims{}); err != nil {
		return fmt.Errorf("invalid jwt returned: %w", err)
	} else if exp, err := j.Claims.GetExpirationTime(); err != nil {
		return fmt.Errorf("jwt was valid but couldn't read expiration: %w", err)
	} else {
		p.token = res.JSON200.Data.Token
		slog.Info("logged in", "claims", j.Claims, "expires", exp.Time)
	}
	return nil
}

func (p *Poller) addAuthHeader(ctx context.Context, req *http.Request) error {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.token))
	return nil
}

func (p *Poller) poll(ctx context.Context) error {
	listDevResp, err := p.Client.GetApiDeviceWithResponse(ctx, &client.GetApiDeviceParams{}, p.addAuthHeader)
	if err != nil {
		return fmt.Errorf("failed to make list devices request: %w", err)
	} else if listDevResp.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status code when listing devices: %d %s", listDevResp.StatusCode(), string(listDevResp.Body))
	}
	for _, resource := range *listDevResp.JSON200.Data {
		if err := p.processDevice(ctx, resource); err != nil {
			return fmt.Errorf("error while polling device: %w", err)
		}
	}

	params := &client.GetApiTimelineParams{}
	if p.lastTimelineId >= 0 {
		params.SinceId = ref.Ref(p.lastTimelineId)
	}
	timelineData, err := p.Client.GetApiTimelineWithResponse(ctx, params, p.addAuthHeader)
	if err != nil {
		return fmt.Errorf("failed to make timeline request: %w", err)
	} else if timelineData.StatusCode() != http.StatusOK {
		return fmt.Errorf("unexpected status code when listing timeline: %d %s", timelineData.StatusCode(), string(timelineData.Body))
	}
	timelineItems := *timelineData.JSON200.Data
	if len(timelineItems) > 0 {
		if p.lastTimelineId == 0 {
			if timelineItems[0].Id != nil {
				p.lastTimelineId = *timelineItems[0].Id
				slog.Info("set initial last timeline id", "id", p.lastTimelineId)
			}
		} else {
			slices.Reverse(timelineItems)
			for _, item := range timelineItems {
				if err := p.processTimelineItem(ctx, item); err != nil {
					return fmt.Errorf("failed to poll timeline item: %w", err)
				}
			}
			p.lastTimelineId = *timelineItems[0].Id
			slog.Info("processed items and set new last timeline id", "#items", len(timelineItems), "id", p.lastTimelineId)
		}
	}

	return nil
}

func ensureGauge(name string, labels map[string]string) (prometheus.Gauge, error) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        name,
		Namespace:   promNamespace,
		ConstLabels: labels,
	})
	if err := prometheus.DefaultRegisterer.Register(gauge); err != nil {
		if conflict := new(prometheus.AlreadyRegisteredError); errors.As(err, conflict) {
			gauge = conflict.ExistingCollector.(prometheus.Gauge)
		} else {
			return nil, fmt.Errorf("failed to register gauge: %w", err)
		}
	}
	return gauge, nil
}

func ensureCounter(name string, labels map[string]string) (prometheus.Counter, error) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        name,
		Namespace:   promNamespace,
		ConstLabels: labels,
	})
	if err := prometheus.DefaultRegisterer.Register(counter); err != nil {
		if conflict := new(prometheus.AlreadyRegisteredError); errors.As(err, conflict) {
			counter = conflict.ExistingCollector.(prometheus.Counter)
		} else {
			return nil, fmt.Errorf("failed to register counter: %w", err)
		}
	}
	return counter, nil
}

func (p *Poller) processDevice(ctx context.Context, resource client.DeviceResource) error {
	deviceLabels := map[string]string{
		"device_id":   strconv.Itoa(ref.DerefOrDefault(resource.Id, 0)),
		"device_name": ref.DerefOrDefault(resource.Name, "unnamed"),
	}

	if resource.LastActivityAt != nil {
		if g, err := ensureGauge("device_last_activity_at_seconds", deviceLabels); err != nil {
			return err
		} else {
			g.Set(float64(resource.LastActivityAt.UTC().Unix()))
		}
	}
	if resource.LastNewEventAt != nil {
		if g, err := ensureGauge("device_last_event_at_seconds", deviceLabels); err != nil {
			return err
		} else {
			g.Set(float64(resource.LastNewEventAt.UTC().Unix()))
		}
	}
	if resource.Status != nil {
		if st, ok := (*resource.Status).(map[string]interface{}); ok {
			bv, ok := st["battery"].(float64)
			if ok {
				if g, err := ensureGauge("device_battery", deviceLabels); err != nil {
					return err
				} else {
					g.Set(bv)
				}
			}

			ov, ok := st["online"].(bool)
			if ok {
				if g, err := ensureGauge("device_online", deviceLabels); err != nil {
					return err
				} else {
					g.Set(map[bool]float64{true: 1, false: 0}[ov])
				}
			}
		}
	}
	return nil
}
func (p *Poller) processTimelineItem(ctx context.Context, item client.TimelineResource) error {
	devicesById := map[int]client.DeviceResource{}
	for _, dev := range ref.DerefOrZero(item.Devices) {
		devicesById[ref.DerefOrZero(dev.Id)] = dev
	}
	for _, wht := range ref.DerefOrZero(item.Weights) {
		dev := devicesById[ref.DerefOrZero(wht.DeviceId)]
		for _, bowl := range ref.DerefOrZero(wht.Frames) {
			if chg := ref.DerefOrZero(bowl.Change); chg != 0 {
				labels := map[string]string{
					"device_id":   strconv.Itoa(ref.DerefOrDefault(dev.Id, 0)),
					"device_name": ref.DerefOrDefault(dev.Name, "unnamed"),
					"event_type":  eventType[ref.DerefOrZero(item.Type)],
				}
				pets := ref.DerefOrZero(item.Pets)
				if len(pets) > 0 {
					pet := pets[0]
					labels["pet_id"] = strconv.Itoa(ref.DerefOrZero(pet.Id))
					labels["pet_name"] = ref.DerefOrDefault(pet.Name, "unnamed")
				}
				if c, err := ensureCounter("weight_change", labels); err != nil {
					return err
				} else {
					c.Add(float64(chg))
				}
			}
		}
	}
	return nil
}

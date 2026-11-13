package homarr

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreateIntegration calls integration.create. homarr tests the connection
// before persisting; when the test fails it answers HTTP 200 with
// {error: ...} and stores nothing, which is returned here as an error that
// carries homarr's response. A result without an integration id is also an
// error, so callers never record an integration that does not exist.
func (c *Client) CreateIntegration(ctx context.Context, intg IntegrationCreate) (Integration, error) {
	var raw json.RawMessage
	if err := c.trpcMutation(ctx, "integration.create", intg, &raw); err != nil {
		return Integration{}, err
	}
	var result integrationCreateResult
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &result); err != nil {
			return Integration{}, fmt.Errorf("integration.create for %s %q: unmarshal result: %w", intg.Kind, intg.Name, err)
		}
	}
	if len(result.Error) > 0 && string(result.Error) != "null" {
		return Integration{}, fmt.Errorf("integration.create for %s %q at %s failed, nothing was persisted: %s",
			intg.Kind, intg.Name, intg.URL, string(result.Error))
	}
	if result.Integration == nil || result.Integration.ID == "" {
		return Integration{}, fmt.Errorf("integration.create for %s %q returned no integration id: %s",
			intg.Kind, intg.Name, string(raw))
	}
	created := *result.Integration
	if result.AppID != nil {
		created.AppID = *result.AppID
	}
	return created, nil
}

func (c *Client) ListIntegrations(ctx context.Context) ([]Integration, error) {
	var result []Integration
	if err := c.trpcQuery(ctx, "integration.all", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) DeleteIntegration(ctx context.Context, id string) error {
	return c.trpcMutation(ctx, "integration.delete", map[string]string{"id": id}, nil)
}

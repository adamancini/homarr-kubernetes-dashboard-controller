package homarr

import (
	"context"
	"fmt"
)

func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App
	if err := c.trpcQuery(ctx, "app.all", nil, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

// CreateApp calls app.create. homarr v2.12.0 returns the app with both id
// and the legacy appId; a result without either is an error so callers never
// record an app that does not exist.
func (c *Client) CreateApp(ctx context.Context, app AppCreate) (App, error) {
	var created struct {
		App
		AppID string `json:"appId"`
	}
	if err := c.trpcMutation(ctx, "app.create", app, &created); err != nil {
		return App{}, err
	}
	if created.ID == "" {
		created.ID = created.AppID
	}
	if created.ID == "" {
		return App{}, fmt.Errorf("app.create for %q returned no app id", app.Name)
	}
	return created.App, nil
}

func (c *Client) UpdateApp(ctx context.Context, id string, app AppUpdate) error {
	return c.trpcMutation(ctx, "app.edit", app, nil)
}

func (c *Client) DeleteApp(ctx context.Context, id string) error {
	return c.trpcMutation(ctx, "app.delete", map[string]string{"id": id}, nil)
}

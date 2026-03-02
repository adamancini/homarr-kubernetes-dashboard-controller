package homarr

import (
	"context"
)

func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	var apps []App
	if err := c.do(ctx, "GET", "/api/apps", nil, &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

func (c *Client) CreateApp(ctx context.Context, app AppCreate) (App, error) {
	var created App
	if err := c.do(ctx, "POST", "/api/apps", app, &created); err != nil {
		return App{}, err
	}
	return created, nil
}

func (c *Client) UpdateApp(ctx context.Context, id string, app AppUpdate) error {
	return c.do(ctx, "PATCH", "/api/apps/"+id, app, nil)
}

func (c *Client) DeleteApp(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/apps/"+id, nil, nil)
}

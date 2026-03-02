package homarr

import "context"

func (c *Client) CreateIntegration(ctx context.Context, intg IntegrationCreate) (Integration, error) {
	var result Integration
	if err := c.trpcMutation(ctx, "integration.create", intg, &result); err != nil {
		return Integration{}, err
	}
	return result, nil
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

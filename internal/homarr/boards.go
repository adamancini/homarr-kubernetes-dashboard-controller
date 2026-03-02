package homarr

import "context"

func (c *Client) CreateBoard(ctx context.Context, board BoardCreate) (Board, error) {
	var result Board
	if err := c.trpcMutation(ctx, "board.createBoard", board, &result); err != nil {
		return Board{}, err
	}
	return result, nil
}

func (c *Client) GetBoardByName(ctx context.Context, name string) (Board, error) {
	var result Board
	if err := c.trpcQuery(ctx, "board.getBoardByName", map[string]string{"name": name}, &result); err != nil {
		return Board{}, err
	}
	return result, nil
}

func (c *Client) SaveBoard(ctx context.Context, board BoardSave) error {
	return c.trpcMutation(ctx, "board.saveBoard", board, nil)
}

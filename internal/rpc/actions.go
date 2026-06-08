package rpc

import "context"

func (c *Client) action(ctx context.Context, method, hash string) error {
	_, err := c.Call(ctx, method, hash)
	return err
}

func (c *Client) Start(ctx context.Context, hash string) error {
	return c.action(ctx, "d.start", hash)
}
func (c *Client) Stop(ctx context.Context, hash string) error {
	return c.action(ctx, "d.stop", hash)
}

// Pause closes the download's files but keeps it in the list.
func (c *Client) Pause(ctx context.Context, hash string) error {
	return c.action(ctx, "d.close", hash)
}
func (c *Client) Recheck(ctx context.Context, hash string) error {
	return c.action(ctx, "d.check_hash", hash)
}
func (c *Client) Announce(ctx context.Context, hash string) error {
	return c.action(ctx, "d.tracker_announce", hash)
}

// Erase removes the torrent from the session (data is left on disk).
func (c *Client) Erase(ctx context.Context, hash string) error {
	return c.action(ctx, "d.erase", hash)
}

// BasePath returns the on-disk path of a download (for optional data deletion).
func (c *Client) BasePath(ctx context.Context, hash string) (string, error) {
	return c.string(ctx, "d.base_path", hash)
}

func (c *Client) SetLabel(ctx context.Context, hash, label string) error {
	_, err := c.Call(ctx, "d.custom1.set", hash, label)
	return err
}
func (c *Client) SetPriority(ctx context.Context, hash string, prio int) error {
	_, err := c.Call(ctx, "d.priority.set", hash, prio)
	return err
}
func (c *Client) SetDirectory(ctx context.Context, hash, dir string) error {
	_, err := c.Call(ctx, "d.directory.set", hash, dir)
	return err
}

// SetGlobalThrottle sets global up/down rate caps in bytes/s (0 = unlimited).
func (c *Client) SetGlobalThrottle(ctx context.Context, down, up int64) error {
	if _, err := c.Call(ctx, "throttle.global_down.max_rate.set", "", down); err != nil {
		return err
	}
	_, err := c.Call(ctx, "throttle.global_up.max_rate.set", "", up)
	return err
}

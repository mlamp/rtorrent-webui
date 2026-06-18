// Server-provided UI config, fetched once on mount from /api/config: the
// optional installation name that brands the tab title + on-screen mark, plus
// capability flags. deleteWithData says whether the server will actually delete
// files from disk, so the SPA knows whether to offer that affordance.
//
// configState is a genuine tri-state so the remove dialog can tell apart
// "still loading" from "loaded and the feature is off" from "config unavailable"
// — each warrants different copy, and none should masquerade as another.
class ConfigState {
  name = $state('')
  deleteWithData = $state(false)
  // browse says the server will serve directory listings (GET /api/fs) confined
  // to the configured download roots, so the Add dialog can wire its save-to
  // combobox. Off when the server can't trust its own filesystem view (TCP daemon
  // without override) or no roots resolve — the field stays plain free-text.
  browse = $state(false)
  configState = $state<'idle' | 'loaded' | 'failed'>('idle')
}

export const config = new ConfigState()

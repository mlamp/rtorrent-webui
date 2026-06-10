// Server-provided UI config, fetched once on mount from /api/version. Currently
// just the optional installation name that brands the tab title + on-screen mark.
class ConfigState {
  name = $state('')
}

export const config = new ConfigState()

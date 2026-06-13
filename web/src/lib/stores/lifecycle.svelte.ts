export type LifecycleSignal = 'show' | 'hide' | 'terminate'

/**
 * Single source of visibility truth. `visible` is the reactive flag consumed by
 * pollWhileVisible; subscribers (the SSE driver) get show/hide/terminate
 * signals SYNCHRONOUSLY from inside the DOM handlers — freeze/pagehide must
 * close the EventSource before the handler returns, and routing through a
 * Svelte $effect would defer past that point.
 *
 * Every handler writes `visible` BEFORE notifying, so the flag can never
 * disagree with the emitted signal regardless of browser event ordering
 * (Chromium fires `resume` before `visibilitychange`).
 */
class Lifecycle {
  visible = $state(typeof document === 'undefined' || document.visibilityState === 'visible')

  private subs = new Set<(s: LifecycleSignal) => void>()
  private remove: (() => void) | null = null

  subscribe(fn: (s: LifecycleSignal) => void): () => void {
    this.subs.add(fn)
    return () => this.subs.delete(fn)
  }

  private notify(s: LifecycleSignal) {
    for (const fn of this.subs) fn(s)
  }

  /** Attach the DOM listeners once; returns a remover (matters for dev/HMR). */
  init(): () => void {
    if (this.remove) return this.remove

    const onVisibility = () => {
      this.visible = document.visibilityState === 'visible'
      this.notify(this.visible ? 'show' : 'hide')
    }
    // Chromium-only Page Lifecycle freeze/resume; inert elsewhere.
    const onFreeze = () => {
      this.visible = false
      this.notify('terminate')
    }
    const onResume = () => {
      this.visible = document.visibilityState === 'visible'
      if (this.visible) this.notify('show')
    }
    // pagehide fires for both bfcache entry (persisted) and real unload; closing
    // the stream here is the documented way to stay bfcache-eligible.
    const onPageHide = () => {
      this.visible = false
      this.notify('terminate')
    }
    const onPageShow = (e: PageTransitionEvent) => {
      if (!e.persisted) return // normal load: onMount connects
      this.visible = document.visibilityState === 'visible'
      if (this.visible) this.notify('show')
    }

    document.addEventListener('visibilitychange', onVisibility)
    const hasFreeze = 'onfreeze' in document
    if (hasFreeze) {
      document.addEventListener('freeze', onFreeze)
      document.addEventListener('resume', onResume)
    }
    window.addEventListener('pagehide', onPageHide)
    window.addEventListener('pageshow', onPageShow)

    this.remove = () => {
      document.removeEventListener('visibilitychange', onVisibility)
      if (hasFreeze) {
        document.removeEventListener('freeze', onFreeze)
        document.removeEventListener('resume', onResume)
      }
      window.removeEventListener('pagehide', onPageHide)
      window.removeEventListener('pageshow', onPageShow)
      this.remove = null
    }
    return this.remove
  }
}

export const lifecycle = new Lifecycle()

import { SvelteSet } from 'svelte/reactivity'

class Selection {
  hashes = new SvelteSet<string>()

  has(h: string) {
    return this.hashes.has(h)
  }
  toggle(h: string) {
    if (this.hashes.has(h)) this.hashes.delete(h)
    else this.hashes.add(h)
  }
  set(h: string, on: boolean) {
    if (on) this.hashes.add(h)
    else this.hashes.delete(h)
  }
  clear() {
    this.hashes.clear()
  }
  replace(hs: string[]) {
    this.hashes.clear()
    for (const h of hs) this.hashes.add(h)
  }
  get size() {
    return this.hashes.size
  }
  list() {
    return [...this.hashes]
  }
}

export const selection = new Selection()

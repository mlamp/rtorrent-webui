import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

/** Merge conditional class names with Tailwind-aware de-duplication. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

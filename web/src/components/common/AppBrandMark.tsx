import { cn } from '@/lib/utils'

/** Decorative mark paired with the product name or an already labelled control. */
export function AppBrandMark({ className }: { className?: string }) {
  return <img
    src="/brand/ta-cosmic-mark.png"
    alt=""
    aria-hidden="true"
    width={512}
    height={319}
    className={cn('shrink-0 object-contain', className)}
    draggable={false}
  />
}

// <model-viewer> is a web component from @google/model-viewer. TypeScript has no way to know
// about custom elements it never sees constructed, so the intrinsic element is declared here
// and typed loosely: only the attributes the viewer actually sets are allowed.
import type * as React from 'react'

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      'model-viewer': React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        src?: string
        alt?: string
        poster?: string
        'camera-controls'?: boolean
        'auto-rotate'?: string | boolean
        'auto-rotate-delay'?: number
        'rotation-per-second'?: string
        'interaction-prompt'?: string
        'shadow-intensity'?: string
        exposure?: string
        'camera-orbit'?: string
        'field-of-view'?: string
        'touch-action'?: string
        ar?: boolean
      }
    }
  }
}

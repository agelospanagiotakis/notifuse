interface LogoProps {
  /** Height in px; the width follows the drawing. */
  height?: number
  className?: string
}

// White-label logo: the deployment overlays /console/logo.png (and icon.png) with the
// operator's brand, so the sidebar shows that instead of an embedded drawing.
export function NotifuseLogo({ height = 22, className }: LogoProps) {
  return (
    <img
      src="/console/logo.png"
      alt="e-Pragma"
      height={height}
      className={`block w-auto ${className ?? ''}`}
      style={{ height, width: 'auto' }}
    />
  )
}

export function NotifuseMark({ height = 24, className }: LogoProps) {
  return (
    <img
      src="/console/icon.png"
      alt="e-Pragma"
      width={height}
      height={height}
      className={`block ${className ?? ''}`}
      style={{ height, width: height }}
    />
  )
}

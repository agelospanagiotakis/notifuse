/**
 * The house palettes, offered first in every colour picker.
 *
 * The editor shipped with Flowbite's generic Tailwind ramps and nothing else, which is how a
 * brand ends up with a fifth grey: two people pick "a nice grey" from 200 swatches and they
 * are not the same grey. These are the values the campaigns actually use, taken from the
 * templates themselves — kois-optics' black bands and beige contact block, e-Pragma's ink and
 * notification pink (the two colours in the wordmark SVG).
 *
 * A workspace-scoped palette would be the next step; this fork serves one company's two
 * properties, so both palettes are listed and labelled.
 */
export interface BrandPalette {
  label: string
  colors: string[]
}

export const KOIS_OPTICS_PALETTE: BrandPalette = {
  label: 'Kois Optics',
  colors: [
    '#111111', // announcement bands, buttons
    '#333333', // body text
    '#656565', // secondary text
    '#E8E1D5', // contact block
    '#FAFAFA', // page background
    '#FFFFFF', // content
    '#4A1A10' // wordmark
  ]
}

export const E_PRAGMA_PALETTE: BrandPalette = {
  label: 'e-Pragma',
  colors: [
    '#42495B', // ink (wordmark, tile)
    '#E91E63', // notification dot
    '#1F2430', // deep ink
    '#F5F6F8', // surface
    '#FFFFFF'
  ]
}

export const BRAND_PALETTES: BrandPalette[] = [KOIS_OPTICS_PALETTE, E_PRAGMA_PALETTE]

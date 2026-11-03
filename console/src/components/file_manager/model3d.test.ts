import { describe, it, expect } from 'vitest'
import { is3DModel } from './model3d'

describe('is3DModel', () => {
  it('recognises the formats the viewer can open', () => {
    expect(is3DModel('glasses.glb')).toBe(true)
    expect(is3DModel('glasses.gltf')).toBe(true)
    expect(is3DModel('Glasses.GLB')).toBe(true) // a camera export often shouts
  })

  it('does not offer a viewer it cannot honour', () => {
    // model-viewer reads glTF only; an .obj or .fbx would open an empty viewer.
    expect(is3DModel('glasses.obj')).toBe(false)
    expect(is3DModel('glasses.fbx')).toBe(false)
    expect(is3DModel('photo.png')).toBe(false)
    expect(is3DModel('glb')).toBe(false) // no extension, no guess
  })
})

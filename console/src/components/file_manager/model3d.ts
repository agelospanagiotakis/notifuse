/**
 * Is this file a 3D model the viewer can show?
 *
 * .glb is the packed binary glTF and .gltf the JSON form; both are what model-viewer reads.
 * Everything else — .obj, .fbx, .stl — needs a conversion first, and offering a button that
 * cannot open the file would be worse than offering none.
 */
export function is3DModel(fileName: string): boolean {
  return /\.(glb|gltf)$/i.test(fileName)
}

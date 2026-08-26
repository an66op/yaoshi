const allowedTypes = new Set(['image/png', 'image/jpeg', 'image/webp'])

function readFile(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(new Error('读取图片失败'))
    reader.readAsDataURL(file)
  })
}

function loadImage(source: string) {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('图片格式无效'))
    image.src = source
  })
}

export async function prepareRoomLogo(file: File) {
  if (!allowedTypes.has(file.type)) throw new Error('请选择 PNG、JPG 或 WebP 图片')
  if (file.size > 5 * 1024 * 1024) throw new Error('原图不能超过 5MB')

  const image = await loadImage(await readFile(file))
  const canvas = document.createElement('canvas')
  const size = 256
  canvas.width = size
  canvas.height = size
  const context = canvas.getContext('2d')
  if (!context) throw new Error('当前浏览器无法处理图片')

  const scale = Math.min(size / image.naturalWidth, size / image.naturalHeight)
  const width = image.naturalWidth * scale
  const height = image.naturalHeight * scale
  context.clearRect(0, 0, size, size)
  context.drawImage(image, (size - width) / 2, (size - height) / 2, width, height)

  const result = canvas.toDataURL('image/webp', .88)
  if (result.length > 500_000) throw new Error('处理后的 Logo 仍然过大，请更换图片')
  return result
}

export async function createEditMarkup(image, selection) {
  const width = image.naturalWidth;
  const height = image.naturalHeight;
  if (!width || !height) throw new Error('请等待原图加载完成后再添加标记。');
  if (width * height > 16777216) throw new Error('OpenAI 局部编辑的底图不能超过 16,777,216 像素。');
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext('2d');
  context.strokeStyle = '#e14947';
  context.lineWidth = Math.max(2, Math.round(Math.min(width, height) / 160));
  const x = selection.x1 / 1000 * width;
  const y = selection.y1 / 1000 * height;
  if (selection.mode === 'point') {
    context.beginPath();
    context.arc(x, y, Math.max(8, Math.min(width, height) / 40), 0, Math.PI * 2);
    context.stroke();
  } else {
    context.strokeRect(x, y, (selection.x2 - selection.x1) / 1000 * width, (selection.y2 - selection.y1) / 1000 * height);
  }
  const blob = await new Promise((resolve, reject) => canvas.toBlob(
    result => result ? resolve(result) : reject(new Error('无法创建局部编辑标记。')), 'image/png',
  ));
  return new File([blob], 'edit-markup.png', { type: 'image/png' });
}

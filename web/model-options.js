import { $ } from './api.js';
import { refreshControls } from './controls.js';

let formatProvider;
let gifMode = false;
const stillSizes = [...$('image-size').options].map(option => [option.value, option.text]);
const stillBackgrounds = [...$('background').options].map(option => [option.value, option.text]);

export function syncModelOptions(openAI, layerMode) {
  $('seedream-resolution').hidden = openAI;
  $('seedream-ratio').hidden = openAI;
  $('seedream-watermark').hidden = openAI;
  $('openai-size').hidden = !openAI;
  $('openai-quality').hidden = !openAI;
  $('openai-complexity').hidden = !openAI;
  $('enable-layers').hidden = openAI || layerMode;
  $('layer-starter').hidden = openAI;
  $('starter-grid').classList.toggle('without-layers', openAI);
  $('file-input').accept = openAI ? 'image/png,image/jpeg,image/webp' : 'image/png,image/jpeg,image/webp,image/bmp,image/tiff,image/gif,image/heic,image/heif,.heic,.heif';
  if (formatProvider !== openAI) {
    const selected = $('format').value;
    const formats = openAI ? ['png', 'jpeg', 'webp', 'gif'] : ['png', 'jpeg'];
    $('format').replaceChildren(...formats.map(format => new Option(format === 'gif' ? 'GIF 动图' : format === 'webp' ? 'WebP' : format.toUpperCase(), format)));
    $('format').value = formats.includes(selected) ? selected : 'png';
    formatProvider = openAI;
  }
  const gif = openAI && $('format').value === 'gif';
  if (gif !== gifMode) {
    const sizes = gif ? [['auto', '尺寸 · 自动'], ['512x512', '512 × 512'], ['768x512', '768 × 512'], ['512x768', '512 × 768'], ['custom', '自定义尺寸']] : stillSizes;
    $('image-size').replaceChildren(...sizes.map(([value, label]) => new Option(label, value)));
    $('image-size').value = 'auto';
    const background = $('background').value;
    const backgrounds = stillBackgrounds.filter(([value]) => !gif || value !== 'transparent');
    $('background').replaceChildren(...backgrounds.map(([value, label]) => new Option(label, value)));
    $('background').value = backgrounds.some(([value]) => value === background) ? background : 'auto';
    gifMode = gif;
  }
  $('gif-options').hidden = !gif;
  $('frame-count').disabled = !gif;
  $('frame-duration').disabled = !gif;
  $('image-width').max = gif ? '' : '3840';
  $('image-height').max = gif ? '' : '3840';
  $('custom-size').hidden = !openAI || $('image-size').value !== 'custom';
  $('image-width').disabled = $('custom-size').hidden;
  $('image-height').disabled = $('custom-size').hidden;
  refreshControls();
}

export function selectedImageSize() {
  return $('image-size').value === 'custom'
    ? `${$('image-width').value}x${$('image-height').value}` : $('image-size').value;
}

export function restoreImageSize(size = 'auto') {
  if ([...$('image-size').options].some(option => option.value === size)) {
    $('image-size').value = size;
  } else {
    $('image-size').value = 'custom';
    [$('image-width').value, $('image-height').value] = size.split('x');
  }
}

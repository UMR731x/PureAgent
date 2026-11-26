import { $ } from './api.js';
import { refreshControls } from './controls.js';

let formatProvider;

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
  $('custom-size').hidden = !openAI || $('image-size').value !== 'custom';
  $('image-width').disabled = $('custom-size').hidden;
  $('image-height').disabled = $('custom-size').hidden;
  $('file-input').accept = openAI ? 'image/png,image/jpeg,image/webp' : 'image/png,image/jpeg,image/webp,image/bmp,image/tiff,image/gif,image/heic,image/heif,.heic,.heif';
  if (formatProvider !== openAI) {
    const selected = $('format').value;
    const formats = openAI ? ['png', 'jpeg', 'webp'] : ['png', 'jpeg'];
    $('format').replaceChildren(...formats.map(format => new Option(format === 'webp' ? 'WebP' : format.toUpperCase(), format)));
    $('format').value = formats.includes(selected) ? selected : 'png';
    formatProvider = openAI;
  }
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

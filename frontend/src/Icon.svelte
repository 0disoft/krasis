<script>
  import monitor from '@phosphor-icons/core/assets/thin/monitor-thin.svg?url';
  import laptop from '@phosphor-icons/core/assets/regular/laptop.svg?url';
  import gear from '@phosphor-icons/core/assets/regular/gear-six.svg?url';
  import warning from '@phosphor-icons/core/assets/regular/warning-circle.svg?url';
  import info from '@phosphor-icons/core/assets/regular/info.svg?url';
  import arrows from '@phosphor-icons/core/assets/regular/arrows-left-right.svg?url';
  import plus from '@phosphor-icons/core/assets/regular/plus.svg?url';
  import clipboard from '@phosphor-icons/core/assets/regular/clipboard.svg?url';
  import update from '@phosphor-icons/core/assets/regular/arrows-clockwise.svg?url';
  import file from '@phosphor-icons/core/assets/regular/file-text.svg?url';
  import shield from '@phosphor-icons/core/assets/regular/shield-check.svg?url';
  import globe from '@phosphor-icons/core/assets/regular/globe.svg?url';
  const assets = { monitor, laptop, 'gear-six': gear, 'warning-circle': warning, info, 'arrows-left-right': arrows, plus, clipboard, 'arrows-clockwise': update, 'file-text': file, 'shield-check': shield, globe };
  let { name, size = 24 } = $props();
  let src = $derived(assets[name]);
</script>
<img class="icon" {src} width={size} height={size} alt="" aria-hidden="true" />

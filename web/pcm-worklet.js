// The AudioContext runs at 16 kHz. Post exactly 100 ms of mono signed PCM16.
class PCMCapture extends AudioWorkletProcessor {
  constructor() { super(); this.samples = new Int16Array(1600); this.offset = 0; }
  process(inputs) {
    const channel = inputs[0]?.[0];
    if (!channel) return true;
    for (const sample of channel) {
      const clamped = Math.max(-1, Math.min(1, sample));
      this.samples[this.offset++] = clamped < 0 ? clamped * 32768 : clamped * 32767;
      if (this.offset === 1600) {
        this.port.postMessage(this.samples.buffer, [this.samples.buffer]);
        this.samples = new Int16Array(1600); this.offset = 0;
      }
    }
    return true;
  }
}
registerProcessor('pcm-capture', PCMCapture);

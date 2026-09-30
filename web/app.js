const $ = id => document.getElementById(id);
let socket, timer, limitTimer, stream, audio, capture, report, finals = [], running = false;
let sent = 0, acknowledged = 0;
const source = $('source');

fetch('/api/config').then(r => r.json()).then(c => { source.options[1].disabled = !c.live_available; }).catch(() => { $('error').textContent = 'Could not read server configuration.'; });
source.addEventListener('change', () => {
  const demo = source.value === 'demo';
  $('start').textContent = demo ? 'Start demo ↗' : 'Start microphone ↗';
  $('mode').textContent = demo ? 'SYNTHETIC DEMO' : 'LIVE · DEEPGRAM';
  $('source-note').textContent = demo ? 'Generated tones + scripted transcript. This demo does not perform speech recognition.' : 'Visible microphone capture. Your audio goes to Deepgram; finalized text goes to the local coaching service.';
  $('consent').checked = false;
});

function releaseAudio() {
  clearInterval(timer); clearTimeout(limitTimer);
  if (capture) { capture.port.onmessage = null; capture.disconnect(); capture = null; }
  stream?.getTracks().forEach(t => t.stop()); stream = null;
  audio?.close().catch(() => {}); audio = null;
  document.body.classList.remove('recording');
}
function idle() {
  releaseAudio(); running = false; $('start').disabled = false; $('stop').disabled = true; source.disabled = false;
}
function fail(message) {
  $('error').textContent = message; $('status').textContent = 'Session stopped';
  idle(); socket?.close();
}
function sendAudio(buffer) {
  if (!running || socket?.readyState !== WebSocket.OPEN) return;
  // Bounded outstanding audio, in addition to browser/network buffering limits.
  if (sent - acknowledged >= 20 || socket.bufferedAmount > 64000) { fail('Connection is too slow. Audio stopped to prevent an unbounded backlog.'); return; }
  socket.send(buffer); sent++;
}
async function beginAudio(current) {
  running = true; $('stop').disabled = false; $('status').textContent = 'Streaming';
  document.body.classList.add('recording');
  limitTimer = setTimeout(finish, 118000);
  if (source.value === 'demo') {
    let chunk = 0;
    timer = setInterval(() => {
      const pcm = new Int16Array(1600);
      for (let i = 0; i < pcm.length; i++) pcm[i] = Math.sin((chunk * 1600 + i) * 2 * Math.PI * 220 / 16000) * 2000;
      sendAudio(pcm.buffer); chunk++;
      if (chunk === 80 && running) finish();
    }, 100);
  } else {
    try {
      const acquired = await navigator.mediaDevices.getUserMedia({audio:{channelCount:1,echoCancellation:true},video:false});
      if (!running || socket !== current) { acquired.getTracks().forEach(t => t.stop()); return; }
      stream = acquired;
      audio = new AudioContext({sampleRate:16000});
      if (audio.sampleRate !== 16000) throw new Error('This browser cannot capture at 16 kHz. Try a different browser.');
      await audio.audioWorklet.addModule('/pcm-worklet.js');
      if (!running || socket !== current) return;
      capture = new AudioWorkletNode(audio, 'pcm-capture');
      capture.port.onmessage = event => sendAudio(event.data);
      audio.createMediaStreamSource(stream).connect(capture);
      // Worklet has silent output; connecting keeps capture scheduled without playback.
      capture.connect(audio.destination); await audio.resume();
    } catch (e) { if (socket === current) fail('Microphone unavailable: ' + e.message); }
  }
}
function finish() {
  if (!running) return;
  releaseAudio(); running = false; $('stop').disabled = true; $('status').textContent = 'Reflecting…';
  if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({type:'stop'}));
}
$('stop').addEventListener('click', finish);
$('start').addEventListener('click', () => {
  if (!$('consent').checked) { $('error').textContent = 'Please confirm consent before starting.'; return; }
  $('error').textContent = ''; $('transcript').replaceChildren(); $('interim').textContent = '';
  $('tip').textContent = 'Listening for your answer…'; $('download').disabled = true; report = null; finals = []; sent = acknowledged = 0;
  for (const id of ['words','fillers','coach-ms']) $(id).textContent = '—';
  $('signals').querySelectorAll('span').forEach(x => x.textContent = '—');
  $('chunks').textContent = '0 chunks received'; $('seconds').textContent = '0.0s audio';
  $('start').disabled = true; source.disabled = true; $('status').textContent = 'Connecting…';
  socket = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`);
  const current = socket;
  socket.onopen = () => current.send(JSON.stringify({type:'start',source:source.value,consent:true,sample_rate:16000}));
  socket.onmessage = event => {
    if (socket !== current) return;
    const data = JSON.parse(event.data);
    if (data.type === 'ready') beginAudio(current);
    if (data.type === 'ack') { acknowledged = data.seq; $('chunks').textContent = `${data.seq} chunks received`; $('seconds').textContent = `${data.audio_seconds.toFixed(1)}s audio`; }
    if (data.type === 'transcript') {
      if (data.final) { finals.push(data.text); const p = document.createElement('p'); p.textContent = data.text; $('transcript').append(p); $('transcript').scrollTop = $('transcript').scrollHeight; $('interim').textContent = ''; }
      else $('interim').textContent = data.text;
    }
    if (data.type === 'error') fail(data.message);
    if (data.type === 'complete') {
      report = {...data,transcript:finals.join(' ')};
      const f = data.feedback;
      $('words').textContent = f.word_count; $('fillers').textContent = f.filler_count; $('coach-ms').textContent = `${data.telemetry.coach_roundtrip_ms}ms`;
      $('tip').textContent = f.tips.join(' ');
      Array.from($('signals').children).forEach((el, i) => { el.querySelector('span').textContent = f.structure_signals[['Situation','Task','Action','Result'][i]] ? '✓ cue' : '—'; });
      $('download').disabled = false; $('status').textContent = 'Complete'; idle();
    }
  };
  socket.onerror = () => { if (socket === current) fail('Could not connect to the audio gateway.'); };
  socket.onclose = () => { if (socket !== current) return; if (!report && !$('error').textContent) $('error').textContent = 'Connection ended before coaching completed.'; idle(); };
});
$('download').addEventListener('click', () => {
  if (!report) return;
  const url = URL.createObjectURL(new Blob([JSON.stringify(report,null,2)],{type:'application/json'}));
  const link = document.createElement('a'); link.href = url; link.download = 'voice-lab-session.json'; link.click(); setTimeout(() => URL.revokeObjectURL(url),1000);
});
window.addEventListener('pagehide', () => { releaseAudio(); socket?.close(); });

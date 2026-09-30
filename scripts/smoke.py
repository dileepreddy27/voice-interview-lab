"""Run against the real local services: python scripts/smoke.py [ws://host:port/ws]."""
import asyncio
import json
import sys
import time

from websockets.asyncio.client import connect


async def main():
    url = sys.argv[1] if len(sys.argv) > 1 else 'ws://127.0.0.1:8080/ws'
    started = time.perf_counter()
    async with connect(url, proxy=None) as ws:
        await ws.send(json.dumps({'type':'start','consent':True,'source':'demo','sample_rate':16000}))
        assert json.loads(await ws.recv())['type'] == 'ready'
        for _ in range(80):
            await ws.send(bytes(3200))
        await ws.send(json.dumps({'type':'stop'}))
        acks, finals = 0, []
        while True:
            event = json.loads(await asyncio.wait_for(ws.recv(), 10))
            if event['type'] == 'error':
                raise RuntimeError(event)
            if event['type'] == 'ack':
                acks += 1
                assert event['seq'] == acks
            if event['type'] == 'transcript' and event['final']:
                finals.append(event['text'])
            if event['type'] == 'complete':
                assert acks == 80 and len(finals) == 4
                assert event['synthetic'] is True
                assert event['telemetry']['audio_bytes'] == 256000
                assert event['telemetry']['audio_seconds'] == 8
                assert event['feedback']['engine'] == 'local-heuristic-v1'
                assert all(event['feedback']['structure_signals'].values())
                print(json.dumps({'verified':'go-python-websocket-demo','wall_ms':round((time.perf_counter()-started)*1000,2),'result':event},indent=2))
                break


asyncio.run(main())

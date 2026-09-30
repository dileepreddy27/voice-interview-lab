"""Dedicated CI Redis DB; verifies stored fields and TTL without flushing shared data."""
import json
import os
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import redis
from coach.service import evaluate, record_metrics

os.environ['REDIS_URL'] = 'redis://127.0.0.1:6379/15'
client = redis.Redis.from_url(os.environ['REDIS_URL'])
before = set(client.scan_iter('voice:metrics:*'))
assert record_metrics(evaluate('Private test transcript', 2)) == 'stored-anonymous-metrics'
created = set(client.scan_iter('voice:metrics:*')) - before
assert len(created) == 1
for key in created:
    try:
        assert set(json.loads(client.get(key))) == {'word_count', 'words_per_minute', 'filler_count'}
        assert 3590 <= client.ttl(key) <= 3600
    finally:
        client.delete(key)
client.close()
print('Redis metrics fields and 1-hour expiry verified')

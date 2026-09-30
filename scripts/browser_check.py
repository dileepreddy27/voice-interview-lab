"""Real-browser demo acceptance check; optional SCREENSHOT_DIR saves screenshots."""
import json
import os
from pathlib import Path
from playwright.sync_api import sync_playwright, expect


with sync_playwright() as p:
    browser = p.chromium.launch()
    page = browser.new_page(viewport={'width': 1440, 'height': 1100}, device_scale_factor=1)
    errors = []
    page.on('pageerror', lambda error: errors.append(str(error)))
    page.goto(os.getenv('VOICE_LAB_URL', 'http://127.0.0.1:8080'))
    page.get_by_role('button', name='Start demo').click()
    expect(page.get_by_role('alert')).to_contain_text('confirm consent')
    page.get_by_role('checkbox').check()
    page.get_by_role('button', name='Start demo').click()
    expect(page.locator('#status')).to_have_text('Complete', timeout=20000)
    expect(page.locator('#chunks')).to_have_text('80 chunks received')
    expect(page.locator('#transcript p')).to_have_count(4)
    expect(page.locator('#signals')).to_contain_text('cue')
    expect(page.locator('#error')).to_be_empty()
    with page.expect_download() as download:
        page.get_by_role('button', name='Download session JSON').click()
    report = json.loads(Path(download.value.path()).read_text())
    assert report['synthetic'] is True
    assert report['telemetry']['audio_bytes'] == 256000
    assert report['transcript'].endswith('production measurements.')
    directory = os.getenv('SCREENSHOT_DIR')
    if directory:
        Path(directory).mkdir(parents=True, exist_ok=True)
        page.screenshot(path=str(Path(directory) / 'dashboard.png'), full_page=True)
    page.set_viewport_size({'width':390,'height':844})
    assert page.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'mobile overflow'
    if directory:
        page.screenshot(path=str(Path(directory) / 'mobile.png'), full_page=True)
    # A fresh run must clear the prior transcript and results.
    page.get_by_role('button', name='Start demo').click()
    expect(page.locator('#transcript p')).to_have_count(0)
    expect(page.locator('#stop')).to_be_enabled()
    page.get_by_role('button', name='Finish answer').click()
    expect(page.locator('#start')).to_be_enabled(timeout=10000)
    assert not errors, errors
    print('Browser verified: consent, demo completion, JSON export, reset, early stop, mobile layout; no JS errors')
    browser.close()

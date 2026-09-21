import asyncio
from pathlib import Path
from playwright.async_api import async_playwright

async def main():
    async with async_playwright() as p:
        context = await p.chromium.launch_persistent_context(
            user_data_dir=str(Path(__file__).parent / "user_data"),
            headless=False,
            proxy={"server": "http://127.0.0.1:10809"},
            user_agent=("Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                        "AppleWebKit/537.36 (KHTML, like Gecko) "
                        "Chrome/120.0 Safari/537.36"),
            viewport={"width": 1440, "height": 900},
            args=["--disable-blink-features=AutomationControlled", "--start-maximized",
                  "--remote-debugging-port=9235", "--remote-debugging-address=127.0.0.1"],
        )
        page = context.pages[0] if context.pages else await context.new_page()
        response = await page.goto("https://www.binance.com/en/square", wait_until="domcontentloaded", timeout=60000)
        await page.wait_for_timeout(2000)
        print(f"MANUAL_VERIFY_READY status={response.status if response else None} title={await page.title()}", flush=True)
        await asyncio.Event().wait()

asyncio.run(main())

"use client";

import { useEffect, useState } from "react";
import QRCode from "qrcode";

const BTC_ADDRESS = "BC1QXP8MC3CRQSYRU6VZUMPPMTLU0KMNZ92SGMN9G9";
const BTC_URI = `bitcoin:${BTC_ADDRESS}`;

export function BuyMeCoffee(): React.ReactElement {
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);

  useEffect(() => {
    QRCode.toDataURL(BTC_URI, { width: 160, margin: 1 })
      .then(setQrDataUrl)
      .catch(() => setQrDataUrl(null));
  }, []);

  return (
    <div className="flex flex-col items-center gap-2 pb-10">
      <p className="text-sm text-zinc-500">☕ Buy me a coffee</p>
      <a href={BTC_URI} className="rounded-lg border border-black/10 dark:border-white/15 p-2 bg-white">
        {qrDataUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={qrDataUrl} alt="Bitcoin donation QR code" width={160} height={160} />
        ) : (
          <div className="w-40 h-40" />
        )}
      </a>
      <span className="text-xs text-zinc-400 font-mono break-all text-center max-w-xs">
        {BTC_ADDRESS}
      </span>
    </div>
  );
}

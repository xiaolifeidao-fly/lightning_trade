import * as crypto from 'crypto';

const TOTP_PERIOD = 30;
const TOTP_DIGITS = 6;

export function generateGoogleAuthCode(secret: string, at: Date = new Date()): string {
  const normalized = normalizeBase32Secret(secret);
  if (!normalized) throw new Error('google auth secret 不能为空');

  const key = base32Decode(normalized);
  const counter = Math.floor(at.getTime() / 1000 / TOTP_PERIOD);

  const msg = Buffer.alloc(8);
  // write uint64 big-endian
  const hi = Math.floor(counter / 0x100000000);
  const lo = counter >>> 0;
  msg.writeUInt32BE(hi, 0);
  msg.writeUInt32BE(lo, 4);

  const hmac = crypto.createHmac('sha1', key);
  hmac.update(msg);
  const sum = hmac.digest();

  const offset = sum[sum.length - 1] & 0x0f;
  const code =
    ((sum[offset] & 0x7f) << 24) |
    ((sum[offset + 1] & 0xff) << 16) |
    ((sum[offset + 2] & 0xff) << 8) |
    (sum[offset + 3] & 0xff);

  const mod = Math.pow(10, TOTP_DIGITS);
  const value = code % mod;
  return String(value).padStart(TOTP_DIGITS, '0');
}

function normalizeBase32Secret(secret: string): string {
  return secret.toUpperCase().replace(/\s/g, '').replace(/-/g, '');
}

function base32Decode(s: string): Buffer {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = 0;
  let value = 0;
  const output: number[] = [];

  for (const ch of s) {
    const idx = alphabet.indexOf(ch);
    if (idx === -1) throw new Error(`非法 base32 字符: ${ch}`);
    value = (value << 5) | idx;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      output.push((value >>> bits) & 0xff);
    }
  }

  return Buffer.from(output);
}

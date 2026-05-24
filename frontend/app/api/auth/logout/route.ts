import { NextRequest, NextResponse } from 'next/server';

const GATEWAY_URL = process.env.GATEWAY_URL || 'http://localhost:1440';

export async function POST(request: NextRequest) {
  const token = request.cookies.get('qwenpi_key')?.value;

  try {
    if (token) {
      await fetch(`${GATEWAY_URL}/api/auth/logout`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      });
    }
  } catch {
    // ignore — cookies on the frontend are cleared regardless
  }

  const response = NextResponse.json({ success: true });
  response.cookies.delete('qwenpi_key');
  response.cookies.delete('qwenpi_apikey');
  return response;
}

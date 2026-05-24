import { NextRequest, NextResponse } from 'next/server';

const GATEWAY_URL = process.env.GATEWAY_URL || 'http://localhost:1440';
const COOKIE_MAX_AGE = 60 * 60 * 24 * 30;

export async function GET(request: NextRequest) {
  const key = request.cookies.get('qwenpi_key')?.value;
  if (key) return NextResponse.json({ authenticated: true });
  return NextResponse.json({ authenticated: false }, { status: 401 });
}

export async function POST(request: NextRequest) {
  const { key } = await request.json();

  if (!key) {
    return NextResponse.json({ success: false, error: 'Key is required' }, { status: 400 });
  }

  try {
    const backendRes = await fetch(`${GATEWAY_URL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key }),
    });

    if (!backendRes.ok) {
      return NextResponse.json({ success: false, error: 'Invalid admin key' }, { status: 401 });
    }

    const body = await backendRes.json();
    const token = body.token as string | undefined;
    const apikey = body.apikey as string | undefined;
    if (!token || !apikey) {
      return NextResponse.json({ success: false, error: 'Backend response missing credentials' }, { status: 502 });
    }

    const response = NextResponse.json({ success: true });
    response.cookies.set('qwenpi_key', token, {
      httpOnly: true,
      secure: false,
      sameSite: 'lax',
      maxAge: COOKIE_MAX_AGE,
      path: '/',
    });
    response.cookies.set('qwenpi_apikey', apikey, {
      httpOnly: true,
      secure: false,
      sameSite: 'lax',
      maxAge: COOKIE_MAX_AGE,
      path: '/',
    });
    return response;
  } catch {
    return NextResponse.json({ success: false, error: 'Backend connection failed' }, { status: 500 });
  }
}

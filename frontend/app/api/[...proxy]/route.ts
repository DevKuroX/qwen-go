import { NextRequest, NextResponse } from 'next/server';

const GATEWAY_URL = process.env.GATEWAY_URL || 'http://localhost:1440';

async function proxyRequest(request: NextRequest, method: string) {
  const url = `${GATEWAY_URL}${request.nextUrl.pathname}${request.nextUrl.search}`;

  // Split-auth: admin key for /api/admin/*, api key for /api/v1/*. If the caller
  // already supplied a credential header we trust them and forward it verbatim
  // (e.g. external clients hitting through this proxy).
  const adminKey = request.cookies.get('qwenpi_key')?.value;
  const apiKey = request.cookies.get('qwenpi_apikey')?.value;
  const incomingAuth = request.headers.get('authorization') || request.headers.get('x-api-key') || '';
  const isAdminPath = request.nextUrl.pathname.startsWith('/api/admin/');

  const headers: HeadersInit = {};
  if (incomingAuth) {
    headers['Authorization'] = incomingAuth.startsWith('Bearer ') ? incomingAuth : `Bearer ${incomingAuth}`;
  } else if (isAdminPath && adminKey) {
    headers['Authorization'] = `Bearer ${adminKey}`;
  } else if (!isAdminPath && apiKey) {
    headers['Authorization'] = `Bearer ${apiKey}`;
  }
  if (method !== 'GET') headers['Content-Type'] = 'application/json';

  const body = method !== 'GET' ? await request.text() : undefined;

  const res = await fetch(url, {
    method,
    headers,
    body,
  });

  const contentType = res.headers.get('content-type') || '';

  if (contentType.includes('text/event-stream')) {
    return new Response(res.body, {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        'Connection': 'keep-alive',
      },
    });
  }

  const text = await res.text();
  try {
    return NextResponse.json(JSON.parse(text), { status: res.status });
  } catch {
    return new Response(text, { status: res.status });
  }
}

export async function GET(req: NextRequest) { return proxyRequest(req, 'GET'); }
export async function POST(req: NextRequest) { return proxyRequest(req, 'POST'); }
export async function PUT(req: NextRequest) { return proxyRequest(req, 'PUT'); }
export async function DELETE(req: NextRequest) { return proxyRequest(req, 'DELETE'); }

export function isTauriRuntime(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return "__TAURI_INTERNALS__" in (window as Window & { __TAURI_INTERNALS__?: unknown });
}

export async function callGoBackend<T>(path: string, init?: RequestInit): Promise<T> {
  const base = process.env.NEXT_PUBLIC_GO_API_URL ?? "http://127.0.0.1:8080";
  const url = `${base.replace(/\/$/, "")}${path.startsWith("/") ? path : `/${path}`}`;
  const headers = new Headers(init?.headers);
  if (init?.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  try {
    const res = await fetch(url, {
      ...init,
      headers,
    });

    if (!res.ok) {
      let details = "";
      try {
        const payload = (await res.json()) as { message?: string; error?: string };
        details = payload.message || payload.error || "";
      } catch {
        // Ignore non-JSON error bodies.
      }
      throw new Error(details ? `Backend error ${res.status}: ${details}` : `Backend error ${res.status}`);
    }

    return (await res.json()) as T;
  } catch (error) {
    if (error instanceof TypeError) {
      throw new Error(
        `Cannot reach backend at ${base}. Ensure Stratum API is running and origin is allowed (CORS).`,
      );
    }
    if (error instanceof Error) {
      throw error;
    }
    throw new Error(`Failed to reach backend at ${base}`);
  }
}

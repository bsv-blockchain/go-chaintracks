export interface BlockHeader {
  height: number;
  hash: string;
  version: number;
  prevHash: string;
  merkleRoot: string;
  time: number;
  bits: number;
  nonce: number;
}

function readUint32LE(data: Uint8Array, offset: number): number {
  return (
    data[offset] |
    (data[offset + 1] << 8) |
    (data[offset + 2] << 16) |
    (data[offset + 3] << 24)
  ) >>> 0;
}

function toHexLE(data: Uint8Array): string {
  return Array.from(data)
    .reverse()
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

async function sha256(data: Uint8Array): Promise<Uint8Array> {
  const buf = await crypto.subtle.digest("SHA-256", data as unknown as ArrayBuffer);
  return new Uint8Array(buf);
}

async function doubleSha256(data: Uint8Array): Promise<Uint8Array> {
  return sha256(await sha256(data));
}

async function parseHeader(data: Uint8Array, height: number): Promise<BlockHeader> {
  const version = readUint32LE(data, 0);
  const prevHash = toHexLE(data.slice(4, 36));
  const merkleRoot = toHexLE(data.slice(36, 68));
  const time = readUint32LE(data, 68);
  const bits = readUint32LE(data, 72);
  const nonce = readUint32LE(data, 76);
  const hash = toHexLE(await doubleSha256(data));

  return { height, hash, version, prevHash, merkleRoot, time, bits, nonce };
}

export class ChaintracksClient {
  private baseURL: string;
  private eventSource: EventSource | null = null;
  private subscribers: Set<(header: BlockHeader) => void> = new Set();

  constructor(baseURL: string) {
    this.baseURL = baseURL.replace(/\/$/, "");
  }

  async getNetwork(): Promise<string> {
    const resp = await fetch(`${this.baseURL}/v2/network`);
    if (!resp.ok) throw new Error(`Failed to fetch network: ${resp.status}`);
    const data = await resp.json();
    return data.value;
  }

  async getTip(): Promise<BlockHeader> {
    const resp = await fetch(`${this.baseURL}/v2/tip`);
    if (!resp.ok) throw new Error(`Failed to fetch tip: ${resp.status}`);
    const data = await resp.json();
    return data.value;
  }

  async getHeaderByHeight(height: number): Promise<BlockHeader> {
    const resp = await fetch(`${this.baseURL}/v2/header/height/${height}`);
    if (!resp.ok) throw new Error(`Failed to fetch header: ${resp.status}`);
    const data = await resp.json();
    return data.value;
  }

  async getHeaderByHash(hash: string): Promise<BlockHeader> {
    const resp = await fetch(`${this.baseURL}/v2/header/hash/${hash}`);
    if (!resp.ok) throw new Error(`Failed to fetch header: ${resp.status}`);
    const data = await resp.json();
    return data.value;
  }

  async getHeaders(height: number, count: number): Promise<BlockHeader[]> {
    const resp = await fetch(
      `${this.baseURL}/v2/headers?height=${height}&count=${count}`
    );
    if (!resp.ok) throw new Error(`Failed to fetch headers: ${resp.status}`);

    const buffer = await resp.arrayBuffer();
    const data = new Uint8Array(buffer);

    if (data.length % 80 !== 0) {
      throw new Error(`Invalid response length: ${data.length} bytes`);
    }

    const headers: BlockHeader[] = [];
    for (let i = 0; i < data.length; i += 80) {
      headers.push(await parseHeader(data.slice(i, i + 80), height + i / 80));
    }

    return headers;
  }

  subscribe(callback: (header: BlockHeader) => void): () => void {
    this.subscribers.add(callback);

    if (!this.eventSource) {
      this.eventSource = new EventSource(`${this.baseURL}/v2/tip/stream`);
      this.eventSource.onmessage = (event) => {
        try {
          const header = JSON.parse(event.data) as BlockHeader;
          this.subscribers.forEach((cb) => cb(header));
        } catch {
          // Ignore parse errors (e.g., keepalive messages)
        }
      };
      this.eventSource.onerror = () => {
        this.eventSource?.close();
        this.eventSource = null;
      };
    }

    return () => {
      this.subscribers.delete(callback);
      if (this.subscribers.size === 0 && this.eventSource) {
        this.eventSource.close();
        this.eventSource = null;
      }
    };
  }

  close(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    this.subscribers.clear();
  }
}

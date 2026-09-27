import { Operation } from '../types';

/**
 * Applies a single OT operation to a string
 */
export function applyOp(doc: string, op: Operation): string {
  const chars = Array.from(doc); // Handles multi-byte characters safely
  if (op.pos > chars.length) {
    op.pos = chars.length;
  }

  if (op.type === 'insert') {
    const textChars = Array.from(op.text || '');
    chars.splice(op.pos, 0, ...textChars);
    return chars.join('');
  } else if (op.type === 'delete') {
    const len = op.len || 0;
    chars.splice(op.pos, len);
    return chars.join('');
  }
  return doc;
}

/**
 * Applies an ordered array of operations to a document string
 */
export function applyOps(doc: string, ops: Operation[]): string {
  let current = doc;
  for (const op of ops) {
    current = applyOp(current, op);
  }
  return current;
}

/**
 * Calculates operation length in characters/runes
 */
export function opLength(op: Operation): number {
  if (op.type === 'insert') {
    return Array.from(op.text || '').length;
  }
  return op.len || 0;
}

/**
 * Transforms two concurrent operations against each other: T(a, b) -> [a', b']
 */
export function transform(a: Operation, b: Operation, priorityA: boolean): [Operation, Operation] {
  const aPrime: Operation = { ...a };
  const bPrime: Operation = { ...b };

  const lenA = opLength(a);
  const lenB = opLength(b);

  if (a.type === 'insert' && b.type === 'insert') {
    if (a.pos < b.pos || (a.pos === b.pos && priorityA)) {
      bPrime.pos += lenA;
    } else {
      aPrime.pos += lenB;
    }
  } else if (a.type === 'insert' && b.type === 'delete') {
    if (a.pos <= b.pos) {
      bPrime.pos += lenA;
    } else if (a.pos >= b.pos + lenB) {
      aPrime.pos -= lenB;
    } else {
      aPrime.pos = b.pos;
      bPrime.len = (bPrime.len || 0) + lenA;
    }
  } else if (a.type === 'delete' && b.type === 'insert') {
    const [tB, tA] = transform(b, a, !priorityA);
    return [tA, tB];
  } else if (a.type === 'delete' && b.type === 'delete') {
    const aEnd = a.pos + (a.len || 0);
    const bEnd = b.pos + (b.len || 0);

    if (aEnd <= b.pos) {
      bPrime.pos -= a.len || 0;
    } else if (bEnd <= a.pos) {
      aPrime.pos -= b.len || 0;
    } else {
      const minPos = Math.min(a.pos, b.pos);
      const maxEnd = Math.max(aEnd, bEnd);

      if (a.pos <= b.pos && aEnd >= bEnd) {
        bPrime.len = 0;
        aPrime.len = maxEnd - minPos - (b.len || 0);
      } else if (b.pos <= a.pos && bEnd >= aEnd) {
        aPrime.len = 0;
        bPrime.len = maxEnd - minPos - (a.len || 0);
      } else if (a.pos < b.pos) {
        const overlap = aEnd - b.pos;
        aPrime.len = (a.len || 0) - overlap;
        bPrime.pos = a.pos;
        bPrime.len = (b.len || 0) - overlap;
      } else {
        const overlap = bEnd - a.pos;
        bPrime.len = (b.len || 0) - overlap;
        aPrime.pos = b.pos;
        aPrime.len = (a.len || 0) - overlap;
      }
    }
  }

  return [aPrime, bPrime];
}

/**
 * Transforms a list of operations A against a list of operations B
 */
export function transformOps(opsA: Operation[], opsB: Operation[], priorityA: boolean): Operation[] {
  let resA = [...opsA];

  for (const opB of opsB) {
    const nextA: Operation[] = [];
    let currB = { ...opB };

    for (const opA of resA) {
      if (currB.type === 'delete' && (currB.len || 0) <= 0) {
        nextA.push(opA);
        continue
      }
      const [transformedA, transformedB] = transform(opA, currB, priorityA);
      if (!(transformedA.type === 'delete' && (transformedA.len || 0) <= 0)) {
        nextA.push(transformedA);
      }
      currB = transformedB;
    }
    resA = nextA;
  }

  return resA;
}

/**
 * Adjusts a local cursor position in response to a remote operation
 */
export function transformCursor(cursor: number, op: Operation): number {
  const len = opLength(op);
  if (op.type === 'insert') {
    if (op.pos <= cursor) {
      return cursor + len;
    }
    return cursor;
  } else if (op.type === 'delete') {
    const deleteEnd = op.pos + len;
    if (cursor <= op.pos) {
      return cursor;
    } else if (cursor >= deleteEnd) {
      return cursor - len;
    } else {
      return op.pos;
    }
  }
  return cursor;
}

/**
 * Computes difference between two strings as a minimal list of insert / delete ops
 */
export function computeDiffAsOps(oldText: string, newText: string): Operation[] {
  if (oldText === newText) return [];

  const oldChars = Array.from(oldText);
  const newChars = Array.from(newText);

  // Common prefix length
  let prefix = 0;
  while (
    prefix < oldChars.length &&
    prefix < newChars.length &&
    oldChars[prefix] === newChars[prefix]
  ) {
    prefix++;
  }

  // Common suffix length
  let oldSuffix = oldChars.length;
  let newSuffix = newChars.length;
  while (
    oldSuffix > prefix &&
    newSuffix > prefix &&
    oldChars[oldSuffix - 1] === newChars[newSuffix - 1]
  ) {
    oldSuffix--;
    newSuffix--;
  }

  const ops: Operation[] = [];

  // Deletion in old string
  const delLen = oldSuffix - prefix;
  if (delLen > 0) {
    ops.push({
      type: 'delete',
      pos: prefix,
      len: delLen,
    });
  }

  // Insertion in new string
  const insChars = newChars.slice(prefix, newSuffix);
  if (insChars.length > 0) {
    ops.push({
      type: 'insert',
      pos: prefix,
      text: insChars.join(''),
    });
  }

  return ops;
}

/**
 * Client-side OT State Machine (Jupiter / ShareDB model)
 */
export class ClientOTState {
  rev: number;
  doc: string;
  pending: Operation[] | null = null; // In flight, awaiting ack
  buffer: Operation[] | null = null;  // Queued local edits while awaiting ack

  constructor(initialDoc: string = '', initialRev: number = 0) {
    this.doc = initialDoc;
    this.rev = initialRev;
  }

  /**
   * Called when user modifies the document locally.
   * Computes ops against current local state, updates local doc, and returns ops to send immediately (if not awaiting ack).
   */
  applyLocalEdit(newDoc: string): Operation[] | null {
    const ops = computeDiffAsOps(this.doc, newDoc);
    if (ops.length === 0) return null;

    this.doc = newDoc;

    if (this.pending === null) {
      // Synchronized -> AwaitingAck
      this.pending = ops;
      return ops;
    } else {
      // AwaitingAck or AwaitingWithBuffer -> Buffer new ops
      if (this.buffer === null) {
        this.buffer = ops;
      } else {
        this.buffer = [...this.buffer, ...ops];
      }
      return null;
    }
  }

  /**
   * Called when server acknowledges the current pending operation batch
   * Returns buffered operations that should now be sent to server, or null.
   */
  handleServerAck(serverRev: number): Operation[] | null {
    this.rev = serverRev;
    this.pending = null;

    if (this.buffer !== null && this.buffer.length > 0) {
      this.pending = this.buffer;
      this.buffer = null;
      return this.pending;
    }

    return null;
  }

  /**
   * Called when an operation arrives from another client via the server.
   * Transforms incoming ops against pending and buffer, updates local doc, and returns new doc and transformed remote ops.
   */
  handleServerOp(serverRev: number, remoteOps: Operation[]): { newDoc: string; transformedRemoteOps: Operation[] } {
    this.rev = serverRev;

    let appliedRemote = [...remoteOps];

    if (this.pending !== null) {
      // Transform incoming remote ops against local in-flight ops
      const newPending = transformOps(this.pending, appliedRemote, true);
      appliedRemote = transformOps(appliedRemote, this.pending, false);
      this.pending = newPending;
    }

    if (this.buffer !== null) {
      // Transform incoming remote ops against local buffer
      const newBuffer = transformOps(this.buffer, appliedRemote, true);
      appliedRemote = transformOps(appliedRemote, this.buffer, false);
      this.buffer = newBuffer;
    }

    // Apply the fully transformed remote ops to the local document
    this.doc = applyOps(this.doc, appliedRemote);

    return {
      newDoc: this.doc,
      transformedRemoteOps: appliedRemote,
    };
  }

  /**
   * Full resynchronization from server
   */
  handleSync(serverRev: number, serverDoc?: string, serverOps?: Operation[]): string {
    this.rev = serverRev;
    if (serverOps && serverOps.length > 0) {
      this.doc = applyOps(this.doc, serverOps);
    } else if (serverDoc !== undefined) {
      this.doc = serverDoc;
    }
    // Clear in-flight state to resync cleanly
    this.pending = null;
    this.buffer = null;
    return this.doc;
  }
}

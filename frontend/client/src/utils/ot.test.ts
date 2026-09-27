import {
  applyOp,
  applyOps,
  transform,
  transformOps,
  computeDiffAsOps,
  transformCursor,
  ClientOTState,
} from './ot';

describe('Operational Transformation Client Engine', () => {
  test('applyOp insert and delete', () => {
    expect(applyOp('Hello World', { type: 'insert', pos: 5, text: ' Beautiful' })).toBe('Hello Beautiful World');
    expect(applyOp('Hello Beautiful World', { type: 'delete', pos: 5, len: 10 })).toBe('Hello World');
  });

  test('computeDiffAsOps generates accurate minimal diff operations', () => {
    const ops = computeDiffAsOps('Hello World', 'Hello Brave World');
    expect(ops).toHaveLength(1);
    expect(ops[0]).toEqual({
      type: 'insert',
      pos: 6,
      text: 'Brave ',
    });

    const docAfter = applyOps('Hello World', ops);
    expect(docAfter).toBe('Hello Brave World');
  });

  test('transform insert vs insert convergence', () => {
    const doc = 'Hello';
    const opA = { type: 'insert' as const, pos: 5, text: ' Alice' };
    const opB = { type: 'insert' as const, pos: 5, text: ' Bob' };

    const [aPrime, bPrime] = transform(opA, opB, true);

    const doc1 = applyOp(applyOp(doc, opA), bPrime);
    const doc2 = applyOp(applyOp(doc, opB), aPrime);

    expect(doc1).toBe(doc2);
    expect(doc1).toBe('Hello Alice Bob');
  });

  test('transformCursor preserves cursor position past inserts and deletes', () => {
    // Cursor at index 10
    // Insert 5 chars at index 2 -> cursor shifts to 15
    expect(transformCursor(10, { type: 'insert', pos: 2, text: '12345' })).toBe(15);
    // Insert 5 chars at index 12 -> cursor stays at 10
    expect(transformCursor(10, { type: 'insert', pos: 12, text: '12345' })).toBe(10);
    // Delete 3 chars before cursor at index 2 -> cursor shifts to 7
    expect(transformCursor(10, { type: 'delete', pos: 2, len: 3 })).toBe(7);
  });

  test('ClientOTState Jupiter state machine lifecycle', () => {
    const client = new ClientOTState('Hello', 0);

    // User types "Hello World" locally
    const ops1 = client.applyLocalEdit('Hello World');
    expect(ops1).not.toBeNull();
    expect(client.pending).toEqual(ops1);
    expect(client.buffer).toBeNull();

    // While awaiting ack, user types "!"
    const ops2 = client.applyLocalEdit('Hello World!');
    expect(ops2).toBeNull(); // Buffered, not sent yet!
    expect(client.buffer).not.toBeNull();

    // Server sends ack for first edit
    const bufferedToSend = client.handleServerAck(1);
    expect(bufferedToSend).not.toBeNull();
    expect(client.pending).toEqual(bufferedToSend);
    expect(client.buffer).toBeNull();

    // Server sends ack for buffered edit
    const nextBuffered = client.handleServerAck(2);
    expect(nextBuffered).toBeNull();
    expect(client.pending).toBeNull();
    expect(client.doc).toBe('Hello World!');
  });
});

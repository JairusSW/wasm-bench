/* SPDX-License-Identifier: MIT
 * Deliberately freestanding: no headers, libc or implicit system includes.
 * Unsigned 32-bit arithmetic defines every shift/overflow in this recurrence.
 */
unsigned int benchmark(unsigned int count) {
    unsigned int state = 0x12345678u;
    for (unsigned int i = 0; i < count; ++i) {
        state ^= state << 13;
        state ^= state >> 17;
        state ^= state << 5;
    }
    return state;
}

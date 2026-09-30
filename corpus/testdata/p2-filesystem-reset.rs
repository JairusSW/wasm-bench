// SPDX-License-Identifier: Apache-2.0
// Every invocation must receive the pristine locked filesystem fixture.
use std::{fs, io::Write};

fn main() {
    assert_eq!(fs::read("/data/input.txt").unwrap(), b"pristine\n");
    let mut sentinel = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open("/data/created.txt")
        .unwrap();
    sentinel.write_all(b"created by guest\n").unwrap();
    fs::write("/data/input.txt", b"mutated by guest\n").unwrap();
    println!("filesystem reset verified");
}

CC = gcc

all: demo

demo: src/main.c
	$(CC) -o demo src/main.c

test: demo
	./demo --self-test


path = []
with open("path.txt", "r") as f:
  lines = f.readlines()
  for row, line in enumerate(lines):
    for col, char in enumerate(line):
      x = row 
      y = col
      if char == "0":
        path.append("{"+str(x)+", " + str(y) + "}")

print(",".join(path))

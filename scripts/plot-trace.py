import csv
import sys

import matplotlib.pyplot as plt
from matplotlib.ticker import MaxNLocator

if len(sys.argv) < 2:
    print("Usage: python plot-trace.py <csv_file>")
    sys.exit(1)

filename = sys.argv[1]

# Read the benchmark results
x = []  # Total changes
y = []  # Average time
with open(filename, "r") as f:
    reader = csv.reader(f)
    next(reader)  # Skip the header
    for row in reader:
        x.append(int(row[0]))
        y.append(float(row[4]))

# Plot the results
plt.figure(figsize=(10, 6))
plt.plot(x, y, label="Average Time per Change")
plt.xlabel("Total Changes")
plt.ylabel("Average Time (Milliseconds)")
plt.title("CRDT Benchmark: Average Time per Change vs Total Changes")
ax = plt.gca()
# 'nbins=20' tells matplotlib to try to fit roughly 20 numbers on the axis
ax.yaxis.set_major_locator(MaxNLocator(nbins=20))
plt.legend()
plt.grid(True)
plt.show()

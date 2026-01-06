## Plan: Build GUI for eBPF Data Visualization

### Objective
Develop a GUI using Go's Bubble Tea framework to visualize kernel-space eBPF data. Focus on creating an extensible and functional interface without integrating with `malloc_trace.py` or optimizing for performance at this stage.

---

### Steps

#### 1. **Set Up Bubble Tea Framework**
- Initialize a new Go project for the GUI.
- Add Bubble Tea as a dependency using:
  ```bash
  go get github.com/charmbracelet/bubbletea
  ```
- Create a basic Bubble Tea application with a placeholder main menu.

#### 2. **Design the GUI Layout**
- **Main View**:
  - Display a scrollable list of processes.
  - Highlight the selected process.
- **Process Details View**:
  - Show kernel-space data for the selected process.
  - Separate sections for:
    - Virtual addresses.
    - Physical PFNs.
  - Include a matrix visualization for addresses.

#### 3. **Implement Process List**
- Create a model to track processes:
  - Process ID, name, start/terminate timestamps.
- Display the process list in the main view.
- Allow user selection with keyboard navigation (e.g., arrow keys).

#### 4. **Implement Process Data View**
- Display kernel-space data for the selected process:
  - Virtual addresses and PFNs.
  - Lifecycle events for virtual addresses.
- Use a grid layout for matrix visualization:
  - Color-code lifecycle phases (creation, initialization, swapped, recovered).

#### 5. **Memory Summation**
- Add a section to display:
  - Total virtual memory usage (sum of virtual addresses, 4KB each).
  - Total physical memory usage (sum of PFNs, 4KB each).

#### 6. **Simulate Data Input**
- Create a mock data generator to simulate kernel-space eBPF data.
- Use the mock data to populate the process list and address details.

#### 7. **Testing and Refinement**
- Test the GUI with mock data.
- Refine the layout and user interactions for clarity and usability.

---

### Further Considerations
1. **Extensibility**: Design the codebase to allow easy addition of new features (e.g., userspace data, advanced visualizations).
2. **Future Integration**: Plan for replacing mock data with Go-based eBPF data collection in the future.
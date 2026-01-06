import java.io.IOException;

public class Test {
    public static void main(String[] args) throws IOException {
        long pid = ProcessHandle.current().pid();
        System.out.printf("PID procesu: %d%n", pid);

        while (true) {
            int ch = System.in.read();
            if (ch == -1) { // EOF lub błąd
                break;
            }

            // Krótkotrwała alokacja bool
            boolean[] b = new boolean[1];
            if (b == null) {
                System.err.println("malloc failed");
                System.exit(1);
            }
            b = null; // oddaj GC
        }
    }
}
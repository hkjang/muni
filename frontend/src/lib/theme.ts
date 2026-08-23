import { alpha, createTheme } from "@mui/material/styles";

export const theme = createTheme({
  cssVariables: true,
  palette: {
    mode: "light",
    primary: { main: "#5151c6", dark: "#3d3da4", light: "#7373d8" },
    secondary: { main: "#167d70" },
    background: { default: "#f6f7fb", paper: "#ffffff" },
    text: { primary: "#22232b", secondary: "#626575" },
    divider: "#e4e5ec",
    success: { main: "#287a58" },
    warning: { main: "#a96412" },
    error: { main: "#c33f49" },
  },
  typography: {
    fontFamily:
      '"Noto Sans KR Variable", "Noto Sans KR", "Malgun Gothic", sans-serif',
    fontSize: 15,
    htmlFontSize: 16,
    h1: { fontSize: "2rem", fontWeight: 720, letterSpacing: "-0.035em" },
    h2: { fontSize: "1.55rem", fontWeight: 700, letterSpacing: "-0.025em" },
    h3: { fontSize: "1.2rem", fontWeight: 680 },
    body1: { fontSize: "1rem", lineHeight: 1.65 },
    body2: { fontSize: "0.9375rem", lineHeight: 1.55 },
    button: { fontSize: "0.9375rem", fontWeight: 650, textTransform: "none" },
  },
  shape: { borderRadius: 10 },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: { minWidth: 320 },
        "*:focus-visible": {
          outline: "3px solid rgba(81,81,198,.35)",
          outlineOffset: 2,
        },
        "*": { scrollbarWidth: "thin", scrollbarColor: "#a8a9ba transparent" },
        "*::-webkit-scrollbar": { width: 10, height: 10 },
        "*::-webkit-scrollbar-track": { background: "transparent" },
        "*::-webkit-scrollbar-thumb": {
          background: "#a8a9ba",
          border: "3px solid transparent",
          backgroundClip: "padding-box",
          borderRadius: 999,
        },
        "*::-webkit-scrollbar-thumb:hover": {
          background: "#828397",
          border: "2px solid transparent",
          backgroundClip: "padding-box",
        },
      },
    },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: { root: { minHeight: 40, borderRadius: 9 } },
    },
    MuiIconButton: {
      styleOverrides: { root: { minWidth: 40, minHeight: 40 } },
    },
    MuiTextField: { defaultProps: { size: "small" } },
    MuiMenuItem: {
      styleOverrides: { root: { minHeight: 42, fontSize: "0.9375rem" } },
    },
    MuiListItemButton: {
      styleOverrides: {
        root: {
          minHeight: 44,
          borderRadius: 9,
          margin: "2px 8px",
          "&.Mui-selected": {
            backgroundColor: alpha("#5151c6", 0.11),
            color: "#3d3da4",
          },
          "&.Mui-selected:hover": { backgroundColor: alpha("#5151c6", 0.16) },
        },
      },
    },
    MuiDialog: { styleOverrides: { paper: { borderRadius: 14 } } },
    MuiCard: {
      styleOverrides: {
        root: {
          border: "1px solid #e4e5ec",
          boxShadow: "0 5px 24px rgba(24,25,38,.045)",
        },
      },
    },
  },
});

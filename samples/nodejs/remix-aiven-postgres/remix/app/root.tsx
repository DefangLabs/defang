import { Links, LiveReload, Meta, Outlet, Scripts } from "@remix-run/react";
import globalStyles from "~/styles/global.css";

export const links = () => [
  { rel: "stylesheet", href: globalStyles },
  { rel: "stylesheet", href: 'https://cdn.jsdelivr.net/npm/@picocss/pico@1/css/pico.min.css' },
];

export default function App() {
  return (
    <html>
      <head>
        <link rel="icon" href="data:image/x-icon;base64,AA" />
        <Meta />
        <Links />
      </head>
      <body data-theme="light">
        <Outlet />

        <Scripts />
        <LiveReload />
      </body>
    </html>
  );
}

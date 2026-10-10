// A page of text, as "правила" and "помощь": only the header and the
// menu are drawn, for a visitor signed in or not.

import { currentUser } from './api.js';
import { renderChrome } from './layout.js';

renderChrome(await currentUser().catch(() => null));

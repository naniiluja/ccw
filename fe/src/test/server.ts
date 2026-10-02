import { setupServer } from 'msw/node'

// Each test registers the handlers it needs; any other request fails the test.
export const server = setupServer()

import type { ProviderInfo } from '@/api/providers'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DefinitionTab } from './definition-tab'
import { ModelsTab } from './models-tab'
import { PolicyTab } from './policy-tab'
import { providerName } from './format'
import { RotationTab } from './rotation-tab'
import { ZenTab } from './zen-tab'

// The OpenCode Zen provider has a session pool of its own.
const zenProviderId = 'opencode'

export function ProviderDetail({ provider }: { provider: ProviderInfo }) {
  const id = provider.id
  return (
    <Card>
      <CardHeader>
        <CardTitle className="truncate">{providerName(provider)}</CardTitle>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="models">
          <div className="max-w-full overflow-x-auto">
            <TabsList>
              <TabsTrigger value="models">Model</TabsTrigger>
              <TabsTrigger value="rotation">Xoay vòng</TabsTrigger>
              <TabsTrigger value="policy">Chính sách model</TabsTrigger>
              <TabsTrigger value="definition">Định nghĩa</TabsTrigger>
              {id === zenProviderId ? (
                <TabsTrigger value="zen">Phiên Zen</TabsTrigger>
              ) : null}
            </TabsList>
          </div>
          <TabsContent value="models" className="mt-4">
            <ModelsTab providerId={id} />
          </TabsContent>
          <TabsContent value="rotation" className="mt-4">
            <RotationTab providerId={id} />
          </TabsContent>
          <TabsContent value="policy" className="mt-4">
            <PolicyTab providerId={id} />
          </TabsContent>
          <TabsContent value="definition" className="mt-4">
            <DefinitionTab provider={provider} />
          </TabsContent>
          {id === zenProviderId ? (
            <TabsContent value="zen" className="mt-4">
              <ZenTab />
            </TabsContent>
          ) : null}
        </Tabs>
      </CardContent>
    </Card>
  )
}

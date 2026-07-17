import Root from './tabs.svelte';
import Content from './tabs-content.svelte';
import List from './tabs-list.svelte';
import Trigger from './tabs-trigger.svelte';
export { tabsListVariants, type TabsListVariant } from './tabs-list-variants.js';

export {
	Root,
	Content,
	List,
	Trigger,
	//
	Root as Tabs,
	Content as TabsContent,
	List as TabsList,
	Trigger as TabsTrigger
};

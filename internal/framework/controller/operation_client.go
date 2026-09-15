package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// operationClient is private to one pass. It refreshes intent before every API
// request, including conflict retries and PVC receipt writes. It cannot revoke
// requests already sent, or make the CR check and child request atomic.
type operationClient struct {
	client.Client
	check func(context.Context) error
}

func checkOperation(ctx context.Context, c client.Client) error {
	if guarded, ok := c.(*operationClient); ok {
		return guarded.check(ctx)
	}
	return nil
}

func (c *operationClient) Get(ctx context.Context, key client.ObjectKey, object client.Object,
	options ...client.GetOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.Get(ctx, key, object, options...)
}

func (c *operationClient) List(ctx context.Context, objects client.ObjectList,
	options ...client.ListOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.List(ctx, objects, options...)
}

func (c *operationClient) Create(ctx context.Context, object client.Object,
	options ...client.CreateOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.Create(ctx, object, options...)
}

func (c *operationClient) Update(ctx context.Context, object client.Object,
	options ...client.UpdateOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.Update(ctx, object, options...)
}

func (c *operationClient) Patch(ctx context.Context, object client.Object, patch client.Patch,
	options ...client.PatchOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.Patch(ctx, object, patch, options...)
}

func (c *operationClient) Delete(ctx context.Context, object client.Object,
	options ...client.DeleteOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.Delete(ctx, object, options...)
}

func (c *operationClient) DeleteAllOf(ctx context.Context, object client.Object,
	options ...client.DeleteAllOfOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.DeleteAllOf(ctx, object, options...)
}

func (c *operationClient) Apply(ctx context.Context, object runtime.ApplyConfiguration,
	options ...client.ApplyOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.Client.Apply(ctx, object, options...)
}

func (c *operationClient) Status() client.SubResourceWriter {
	return &operationWriter{SubResourceWriter: c.Client.Status(), check: c.check}
}
func (c *operationClient) SubResource(name string) client.SubResourceClient {
	raw := c.Client.SubResource(name)
	return &operationSubresource{operationWriter: operationWriter{SubResourceWriter: raw, check: c.check}, reader: raw}
}

type operationWriter struct {
	client.SubResourceWriter
	check func(context.Context) error
}

func (c *operationWriter) Create(ctx context.Context, object, subresource client.Object,
	options ...client.SubResourceCreateOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.SubResourceWriter.Create(ctx, object, subresource, options...)
}

func (c *operationWriter) Update(ctx context.Context, object client.Object,
	options ...client.SubResourceUpdateOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.SubResourceWriter.Update(ctx, object, options...)
}

func (c *operationWriter) Patch(ctx context.Context, object client.Object, patch client.Patch,
	options ...client.SubResourcePatchOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.SubResourceWriter.Patch(ctx, object, patch, options...)
}

func (c *operationWriter) Apply(ctx context.Context, object runtime.ApplyConfiguration,
	options ...client.SubResourceApplyOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.SubResourceWriter.Apply(ctx, object, options...)
}

type operationSubresource struct {
	operationWriter
	reader client.SubResourceReader
}

func (c *operationSubresource) Get(ctx context.Context, object, subresource client.Object,
	options ...client.SubResourceGetOption,
) error {
	if err := c.check(ctx); err != nil {
		return err
	}
	return c.reader.Get(ctx, object, subresource, options...)
}
